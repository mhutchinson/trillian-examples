package witness

import "errors"

type Witness struct {
	db *Database
}

func NewWitness(db *Database) *Witness {
	return &Witness{
		db: db,
	}
}

// GetLatest gets the latest Checkpoint.
// This will be consistent with all other Checkpoints returned.
func (w *Witness) GetLatest() (Checkpoint, error) {
	return w.db.Checkpoint()
}

// Update updates latest Checkpoint if `from` matches the current version.
// Both Checkpoints will be checked for authenticity from the Log.
// If `to` > `from` and the proof shows valid append-only property,
// the Golden Checkpoint will be updated.
func (w *Witness) Update(from, to Checkpoint, proof ConsistencyProof) error {
	return errors.New("update not implemented")
}

// Registers a new log with the TOFU Checkpoint and the given key
// This Checkpoint must be signed by the PubKey
// All updates applied via Exchange will be verified with this key
func (w *Witness) Init(Checkpoint, PubKey) error {
	return errors.New("Init not implemented")
}
