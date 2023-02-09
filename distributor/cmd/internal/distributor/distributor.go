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
	"context"
	"errors"
)

type Distributor struct {
	// Maybe use note.Verifiers
}

// GetLogs returns a list of all logs the distributor is aware of.
func (d *Distributor) GetLogs() ([]string, error) {
	return nil, errors.New("not implemented")
}

// GetCheckpointN gets a checkpoint for a given log, which is consistent with all
// other checkpoints for the same log signed by this witness.
func (d *Distributor) GetCheckpointN(logID string, sigs uint32) ([]byte, error) {
	return nil, errors.New("not implemented")
}

// GetCheckpointWitness gets a checkpoint for a given log, witnessed by the given witness.
func (d *Distributor) GetCheckpointWitness(logID, witID string) ([]byte, error) {
	return nil, errors.New("not implemented")
}

// Distribute adds a new signed checkpoint to be distributed.
func (d *Distributor) Distribute(ctx context.Context, logID, witID string, nextRaw []byte) error {
	return errors.New("not implemented")
}
