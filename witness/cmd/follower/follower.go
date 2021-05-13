// follower is a witness which is made available via a RESTful API, and automatically follows
// a given log to keep itself up to date.
package main

import (
	"flag"
	"time"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/witness/internal/witness"
)

var (
	logUrl       = flag.String("log", "http://localhost:8000", "Base URL of log server")
	pollInterval = flag.Duration("poll_interval", 5*time.Second, "Duration to wait between polling for new entries")
)

func main() {
	flag.Parse()

	var db *witness.Database // This needs to be initialized by passing in a sqlite file or DB connection string, etc.

	witness := witness.NewWitness(db)

	var cp witness.Checkpoint
	var err error
	if cp, err = witness.GetLatest(); err != nil {
		glog.Exitf("failed to get latest checkpoint: %v", err)
	} else if cp.TreeSize == 0 {
		// Go to the log to get the latest checkpoint
		// key will probably be passed as an argument
		witness.Init(latestCp, key)
	}

	// Now the witness core is initialized, we just need to keep it up to date
	for {
		// Look at LogFollower in the FT example for some inspiration here.
		// Maybe even make that generic.
		var nextCp witness.Checkpoint
		var proof witness.ConsistencyProof // Get the proof from cp to nextCp from the log

		witness.Update(cp, nextCp, proof)
		cp = nextCp
	}
}
