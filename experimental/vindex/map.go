// Copyright 2025 Google LLC. All Rights Reserved.
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

// vindex contains a prototype of an in-memory verifiable index.
// This version uses the clone tool DB as the log source.
package vindex

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/google/trillian-examples/clone/logdb"
	"k8s.io/klog/v2"
)

// MapFn takes the raw leaf data from a log entry and outputs the SHA256 hashes
// of the keys at which this leaf should be indexed under.
// A leaf can be recorded at any number of entries, including no entries (in which case an empty slice must be returned).
type MapFn func([]byte) [][]byte

func NewIndexBuilder(ctx context.Context, log *logdb.Database, mapFn MapFn, walPath string) (IndexBuilder, error) {
	b := IndexBuilder{
		log:   log,
		mapFn: mapFn,
		wal: &writeAheadLog{
			walPath: walPath,
		},
	}
	return b, b.init(ctx)
}

// IndexBuilder pulls data from a clone log DB, applies a MapFn, and outputs
// the resulting operations needed on the map to a write-ahead log.
type IndexBuilder struct {
	log   *logdb.Database
	mapFn MapFn
	wal   *writeAheadLog
}

func (b IndexBuilder) init(ctx context.Context) error {
	// Ready the write-ahead log, to determine the index we can guarantee we processed
	idx, err := b.wal.init()
	if err != nil {
		return err
	}

	// Kick off a thread to read from the DB from the index onwards and:
	//  - update the WAL
	//  - announce new updates via a channel (TODO)

	go b.pullFromDatabase(ctx, idx)

	// Kick off a thread to:
	//  - read snapshot of the WAL and populate map
	//  - consume entries from the channel to update the map

	return nil
}

func (b IndexBuilder) pullFromDatabase(ctx context.Context, start uint64) {
	size, rawCp, _, err := b.log.GetLatestCheckpoint(ctx)
	if err != nil {
		klog.Exitf("Panic: failed to get latest checkpoint from DB: %v", err)
	}

	if size > start {
		leaves := make(chan logdb.StreamResult, 1)
		b.log.StreamLeaves(ctx, start, size, leaves)

		for i := start; i < size; i++ {
			l := <-leaves
			if l.Err != nil {
				klog.Exitf("Panic: failed to read leaf at index %d: %v", i, err)
			}
			hashes := b.mapFn(l.Leaf)
			if err := b.wal.append(i, hashes); err != nil {
				klog.Exitf("failed to add index to entry for leaf %d: %v", i, err)
			}
			// TODO(mhutchinson): announce updates to map construction code
		}
	}

	// TODO(mhutchinson): the raw log checkpoint needs to be propagated into the map checkpoint
	_ = rawCp
}

type writeAheadLog struct {
	walPath string

	entries []string
}

// init reads the file and determines what the last mapped log index was, and returns it.
// This method populates entries with the lines from the WAL up to and including the last
// good entry. The assumption is that all lines ending with a newline were written correctly.
func (l *writeAheadLog) init() (uint64, error) {
	f, err := os.Open(l.walPath)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = f.Close()
	}()

	r := bufio.NewReader(f)

	l.entries = make([]string, 0, 64)
	for {
		var line []byte
		line, err = r.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				// Don't append any trailing line that doesn't end with newline
				break
			}
			return 0, err
		}
		// strip off the newline
		l.entries = append(l.entries, string(line[:len(line)-1]))
	}

	if len(l.entries) == 0 {
		return 0, nil
	}
	lastEntry := l.entries[len(l.entries)-1]
	idx, _, err := unmarshalWalEntry(lastEntry)
	return idx, err
}

func (l *writeAheadLog) append(idx uint64, hashes [][]byte) error {
	e, err := marshalWalEntry(idx, hashes)
	if err != nil {
		return fmt.Errorf("failed to marshal entry: %v", err)
	}
	// TODO(mhutchinson): write out the entry
	_ = e
	return nil
}

// unmarshalWalEntry parses a line from the WAL.
// This is the reverse of marshalWalEntry.
func unmarshalWalEntry(e string) (uint64, [][]byte, error) {
	tokens := strings.Split(e, " ")
	log.Print(e)
	idx, err := strconv.ParseUint(tokens[0], 10, 64)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to parse idx from %q", e)
	}

	hashes := make([][]byte, 0, len(tokens)-1)
	for i, h := range tokens[1:] {
		parsed, err := hex.DecodeString(h)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to parse hex token %d from %q", i, e)
		}
		hashes = append(hashes, parsed)
	}

	return idx, hashes, nil
}

// unmarshalWalEntry converts an index and the hashes it affects into a line for the WAL.
// This is the reverse of unmarshalWalEntry.
func marshalWalEntry(idx uint64, hashes [][]byte) (string, error) {
	sb := strings.Builder{}
	if _, err := sb.WriteString(strconv.FormatUint(idx, 10)); err != nil {
		return "", err
	}
	for _, h := range hashes {
		if _, err := sb.WriteString(" " + hex.EncodeToString(h)); err != nil {
			return "", err
		}
	}
	return sb.String(), nil
}
