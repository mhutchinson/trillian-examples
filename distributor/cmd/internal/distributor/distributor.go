// Copyright 2023 Google LLC. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package distributor is designed to make witnessed checkpoints of verifiable logs
// available to clients in an efficient manner.
package distributor

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"sort"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/formats/checkpoints"
	"github.com/transparency-dev/formats/log"
	"golang.org/x/mod/sumdb/note"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func NewDistributor(ws Witnesses, ls Logs, db *sql.DB) *Distributor {
	return &Distributor{
		ws: ws,
		ls: ls,
		db: db,
	}
}

type Distributor struct {
	ws Witnesses
	ls Logs
	db *sql.DB
}

func (d *Distributor) Init() error {
	_, err := d.db.Exec(`CREATE TABLE IF NOT EXISTS chkpts (
		logID BLOB,
		witID BLOB,
		treeSize INTEGER,
		chkpt BLOB,
		PRIMARY KEY (logID, witID)
		)`)
	return err
}

// GetLogs returns a list of all logs the distributor is aware of.
func (d *Distributor) GetLogs(ctx context.Context) ([]string, error) {
	r := make([]string, 0, len(d.ls))
	for k := range d.ls {
		r = append(r, k)
	}
	sort.Strings(r)
	return r, nil
}

// GetCheckpointN gets a checkpoint for a given log, which is consistent with all
// other checkpoints for the same log signed by this witness.
func (d *Distributor) GetCheckpointN(ctx context.Context, logID string, sigs uint32) ([]byte, error) {
	l, ok := d.ls[logID]
	if !ok {
		return nil, fmt.Errorf("unknown log ID %q", logID)
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	rows, err := tx.QueryContext(ctx, "SELECT treeSize, witID, chkpt FROM chkpts WHERE logID = ? ORDER BY treeSize DESC", logID)
	if err != nil {
		return nil, fmt.Errorf("query failed: %v", err)
	}
	var currentSize uint64
	var witsAtSize []string
	var cpsAtSize [][]byte
	var size uint64
	var widID string
	var cp []byte
	for rows.Next() {
		if err := rows.Scan(&size, &widID, &cp); err != nil {
			return nil, fmt.Errorf("failed to scan rows: %v", err)
		}
		if size != currentSize {
			if len(cpsAtSize) >= int(sigs) {
				witVs := make([]note.Verifier, len(witsAtSize))
				for i, witID := range witsAtSize {
					witVs[i] = d.ws[witID]
				}
				cp, err := checkpoints.Combine(cpsAtSize, l.Verifier, note.VerifierList(witVs...))
				if err != nil {
					return nil, fmt.Errorf("failed to combine sigs: %v", err)
				}
				return cp, nil
			}
			cpsAtSize = make([][]byte, 0)
			currentSize = size
		}
		witsAtSize = append(witsAtSize, widID)
		cpsAtSize = append(cpsAtSize, cp)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan rows: %v", err)
	}
	if len(cpsAtSize) >= int(sigs) {
		witVs := make([]note.Verifier, len(witsAtSize))
		for i, witID := range witsAtSize {
			witVs[i] = d.ws[witID]
		}
		cp, err := checkpoints.Combine(cpsAtSize, l.Verifier, note.VerifierList(witVs...))
		if err != nil {
			return nil, fmt.Errorf("failed to combine sigs: %v", err)
		}
		return cp, nil
	}
	return nil, fmt.Errorf("no checkpoint with %d signatures found", sigs)
}

// GetCheckpointWitness gets a checkpoint for a given log, witnessed by the given witness.
func (d *Distributor) GetCheckpointWitness(ctx context.Context, logID, witID string) ([]byte, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	return getLatestCheckpoint(tx, logID, witID)
}

// Distribute adds a new signed checkpoint to be distributed.
func (d *Distributor) Distribute(ctx context.Context, logID, witID string, nextRaw []byte) error {
	l, ok := d.ls[logID]
	if !ok {
		return fmt.Errorf("unknown log ID %q", logID)
	}
	wv, ok := d.ws[witID]
	if !ok {
		return fmt.Errorf("unknown witness ID %q", witID)
	}
	newCP, _, n, err := log.ParseCheckpoint(nextRaw, l.Origin, l.Verifier, wv)
	if err != nil {
		return fmt.Errorf("failed to parse checkpoint: %v", err)
	}
	if len(n.Sigs) != 2 {
		return fmt.Errorf("failed to verify log and witness signatures; only verified: %v", n.Sigs)
	}

	// This is a valid checkpoint for this log for this witness
	// Now find the previous checkpoint if one exists.

	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	oldBs, err := getLatestCheckpoint(tx, logID, witID)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			// If this is the first checkpoint for this witness then just save and exit
			return d.saveCheckpoint(tx, logID, witID, newCP.Size, nextRaw)
		}
		return err
	}

	// We have the previous checkpoint, now check that the new one is fresher

	oldCP, _, _, err := log.ParseCheckpoint(oldBs, l.Origin, l.Verifier, wv)
	if err != nil {
		// This really shouldn't ever happen unless the DB is corrupted or the config
		// for the log or verifier has changed.
		return err
	}
	if newCP.Size < oldCP.Size {
		return fmt.Errorf("checkpoint for log %q and witness %q is for size %d, cannot update to size %d", logID, witID, oldCP.Size, newCP.Size)
	}
	if newCP.Size == oldCP.Size {
		if !bytes.Equal(newCP.Hash, oldCP.Hash) {
			reportInconsistency(oldBs, nextRaw)
			return fmt.Errorf("old checkpoint for tree size %d had hash %x but new one has %x", newCP.Size, oldCP.Hash, newCP.Hash)
		}
		// Nothing to do; checkpoint is equivalent to the old one so avoid DB writes.
		return nil
	}
	return d.saveCheckpoint(tx, logID, witID, newCP.Size, nextRaw)
}

func (d *Distributor) saveCheckpoint(tx *sql.Tx, logID, witID string, treeSize uint64, cp []byte) error {
	_, err := tx.Exec(`INSERT OR REPLACE INTO chkpts (logID, witID, treeSize, chkpt) VALUES (?, ?, ?, ?)`, logID, witID, treeSize, cp)
	if err != nil {
		return fmt.Errorf("Exec(): %v", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func getLatestCheckpoint(tx *sql.Tx, logID, witID string) ([]byte, error) {
	row := tx.QueryRow("SELECT chkpt FROM chkpts WHERE logID = ? AND witID = ?", logID, witID)
	if err := row.Err(); err != nil {
		return nil, err
	}
	var chkpt []byte
	if err := row.Scan(&chkpt); err != nil {
		if err == sql.ErrNoRows {
			return nil, status.Errorf(codes.NotFound, "no checkpoint for log %q", logID)
		}
		return nil, err
	}
	return chkpt, nil
}

// reportInconsistency makes a note when two checkpoints are found for the same
// log tree size, but with different hashes.
// For now, this simply logs an error, but this could be upgraded to write to a
// new DB table containing this kind of evidence. Care needs to be taken if this
// approach is followed to ensure that the DB size stays limited, i.e. don't allow
// the same/similar inconsistencies to be written indefinitely.
func reportInconsistency(oldCP, newCP []byte) {
	glog.Errorf("Found inconsistent checkpoints:\n%v\n\n%v", oldCP, newCP)
}

type Witnesses map[string]note.Verifier

type Logs map[string]LogInfo

type LogInfo struct {
	Origin   string
	Verifier note.Verifier
}

type AggregatedCheckpoint struct {
	N          uint32
	Checkpoint []byte
}
