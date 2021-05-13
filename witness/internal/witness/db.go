package witness

import "errors"

// Database simply gets and puts things into persistent storage.
type Database struct {
}

// Checkpoint reads the latest checkpoint written to the DB.
func (d *Database) Checkpoint() (Checkpoint, error) {
	// TODO: probably query for the checkpoint with the largest tree size.
	return Checkpoint{}, errors.New("not implemented")
}

// SetCheckpoint writes the checkpoint to the DB.
func (d *Database) SetCheckpoint(Checkpoint) error {
	return errors.New("not implemented")
}
