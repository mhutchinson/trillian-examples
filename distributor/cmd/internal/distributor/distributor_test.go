package distributor_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/trillian-examples/distributor/cmd/internal/distributor"
	_ "github.com/mattn/go-sqlite3" // Load drivers for sqlite3
	"github.com/transparency-dev/formats/log"
	"golang.org/x/mod/sumdb/note"
)

var (
	logFoo = fakeLog{
		LogInfo: distributor.LogInfo{
			Origin:   "from foo",
			Verifier: verifierOrDie("FooLog+3d42aea6+Aby03a35YY+FNI4dfRSvLtq1jQE5UjxIW5CXfK0hiIac"),
		},
		signer: signerOrDie("PRIVATE+KEY+FooLog+3d42aea6+AdLOqvyC6Q/86GltHux+trlUT3fRKyCtnc/1VMrmLIdo"),
	}
	logBar = fakeLog{
		LogInfo: distributor.LogInfo{
			Origin:   "from bar",
			Verifier: verifierOrDie("BarLog+74e9e60a+AQXax81tHt0hpLWhLfnmZ677jAQ7+PLWenJqNrj83CeC"),
		},
		signer: signerOrDie("PRIVATE+KEY+BarLog+74e9e60a+AckT6UKhbEXLxB57ZoqJNWRFsUJ+T6hnZrDd7G+SfZ5h"),
	}
	witWhittle = fakeWitness{
		verifier: verifierOrDie("Whittle+0fc7a204+AVcy4ozqLddii0hxKZNAmBiUIv7yFolUC+fUB/O44GLI"),
		signer:   signerOrDie("PRIVATE+KEY+Whittle+0fc7a204+AfzcRAGTc9Lrim47fDQ+elRKfflP92RXAkPqAojYkcaJ"),
	}
	witWattle = fakeWitness{
		verifier: verifierOrDie("Wattle+1c75450a+AYHI4pLRIKv6LEnH+LiozE2HeMUxGXJRVHrg3Nm5UgfY"),
		signer:   signerOrDie("PRIVATE+KEY+Wattle+1c75450a+ASVbnzJKChp9hp1lUGX9ybsUDQK2WQOnLAefGzahraTg"),
	}
)

func TestGetLogs(t *testing.T) {
	ws := distributor.Witnesses{}
	testCases := []struct {
		desc string
		logs distributor.Logs
		want []string
	}{
		{
			desc: "No logs",
			logs: distributor.Logs{},
			want: []string{},
		},
		{
			desc: "One log",
			logs: distributor.Logs{
				"FooLog": logFoo.LogInfo,
			},
			want: []string{"FooLog"},
		},
		{
			desc: "Two logs",
			logs: distributor.Logs{
				"FooLog": logFoo.LogInfo,
				"BarLog": logBar.LogInfo,
			},
			want: []string{"BarLog", "FooLog"},
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			sqlitedb, err := sql.Open("sqlite3", ":memory:")
			if err != nil {
				t.Fatalf("failed to open temporary in-memory DB: %v", err)
			}
			d := distributor.NewDistributor(ws, tC.logs, sqlitedb)
			if err := d.Init(); err != nil {
				t.Fatalf("Init(): %v", err)
			}
			got, err := d.GetLogs(context.Background())
			if err != nil {
				t.Errorf("GetLogs(): %v", err)
			}
			if !cmp.Equal(got, tC.want) {
				t.Errorf("got %q, want %q", got, tC.want)
			}
		})
	}
}

func TestDistributeLogAndWitnessMustMatchCheckpoint(t *testing.T) {
	ws := distributor.Witnesses{
		"Whittle": witWhittle.verifier,
		"Wattle":  witWattle.verifier,
	}
	ls := distributor.Logs{
		"FooLog": logFoo.LogInfo,
		"BarLog": logBar.LogInfo,
	}
	testCases := []struct {
		desc     string
		reqLogID string
		reqWitID string
		log      fakeLog
		wit      fakeWitness
		wantErr  bool
	}{
		{
			desc:     "Correct log and witness: foo and whittle",
			reqLogID: "FooLog",
			reqWitID: "Whittle",
			log:      logFoo,
			wit:      witWhittle,
			wantErr:  false,
		},
		{
			desc:     "Correct log and witness: bar and wattle",
			reqLogID: "BarLog",
			reqWitID: "Wattle",
			log:      logBar,
			wit:      witWattle,
			wantErr:  false,
		},
		{
			desc:     "Correct log wrong witness",
			reqLogID: "FooLog",
			reqWitID: "Whittle",
			log:      logFoo,
			wit:      witWattle,
			wantErr:  true,
		},
		{
			desc:     "Wrong log correct witness",
			reqLogID: "BarLog",
			reqWitID: "Whittle",
			log:      logFoo,
			wit:      witWhittle,
			wantErr:  true,
		},
		{
			desc:     "Wrong log wrong witness",
			reqLogID: "BarLog",
			reqWitID: "Whittle",
			log:      logFoo,
			wit:      witWattle,
			wantErr:  true,
		},
		{
			desc:     "Unknown log known witness",
			reqLogID: "DogNotLog",
			reqWitID: "Wattle",
			log:      logFoo,
			wit:      witWattle,
			wantErr:  true,
		},
		{
			desc:     "Correct log unkown witness",
			reqLogID: "FooLog",
			reqWitID: "WhatAWally",
			log:      logFoo,
			wit:      witWattle,
			wantErr:  true,
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			sqlitedb, err := sql.Open("sqlite3", ":memory:")
			if err != nil {
				t.Fatalf("failed to open temporary in-memory DB: %v", err)
			}
			d := distributor.NewDistributor(ws, ls, sqlitedb)
			if err := d.Init(); err != nil {
				t.Fatalf("Init(): %v", err)
			}

			logCP16 := tC.log.checkpoint(16, "16", tC.wit.signer)
			err = d.Distribute(context.Background(), tC.reqLogID, tC.reqWitID, logCP16)
			if (err != nil) != tC.wantErr {
				t.Errorf("unexpected error output (wantErr: %t): %v", tC.wantErr, err)
			}
		})
	}
}

func TestDistributeEvolution(t *testing.T) {
	// The base case for this test is that a single checkpoint has already
	// been registered for log foo, by whittle, at tree size 16, with root hash H("16").
	ws := distributor.Witnesses{
		"Whittle": witWhittle.verifier,
		"Wattle":  witWattle.verifier,
	}
	ls := distributor.Logs{
		"FooLog": logFoo.LogInfo,
		"BarLog": logBar.LogInfo,
	}
	testCases := []struct {
		desc     string
		log      fakeLog
		wit      fakeWitness
		size     uint64
		hashSeed string
		wantErr  bool
	}{
		{
			desc:     "whittle a bit bigger",
			log:      logFoo,
			wit:      witWhittle,
			size:     18,
			hashSeed: "18",
			wantErr:  false,
		},
		{
			desc:     "whittle smaller",
			log:      logFoo,
			wit:      witWhittle,
			size:     11,
			hashSeed: "11",
			wantErr:  true,
		},
		{
			desc:     "whittle same",
			log:      logFoo,
			wit:      witWhittle,
			size:     16,
			hashSeed: "16",
			wantErr:  false,
		},
		{
			desc:     "whittle same size but different hash",
			log:      logFoo,
			wit:      witWhittle,
			size:     16,
			hashSeed: "not 16",
			wantErr:  true,
		},
		{
			desc:     "whittle smaller different log",
			log:      logBar,
			wit:      witWhittle,
			size:     11,
			hashSeed: "11",
			wantErr:  false,
		},
		{
			desc:     "wattle smaller",
			log:      logFoo,
			wit:      witWattle,
			size:     11,
			hashSeed: "11",
			wantErr:  false,
		},
		{
			desc:     "wattle same size",
			log:      logFoo,
			wit:      witWattle,
			size:     16,
			hashSeed: "16",
			wantErr:  false,
		},
		{
			desc:     "wattle same size but different hash",
			log:      logFoo,
			wit:      witWattle,
			size:     16,
			hashSeed: "not 16",
			wantErr:  false, // We don't check consistency with all witnesses on write
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			sqlitedb, err := sql.Open("sqlite3", ":memory:")
			if err != nil {
				t.Fatalf("failed to open temporary in-memory DB: %v", err)
			}
			d := distributor.NewDistributor(ws, ls, sqlitedb)
			if err := d.Init(); err != nil {
				t.Fatalf("Init(): %v", err)
			}
			err = d.Distribute(context.Background(), "FooLog", "Whittle", logFoo.checkpoint(16, "16", witWhittle.signer))
			if err != nil {
				t.Fatalf("Distribute(): %v", err)
			}

			err = d.Distribute(context.Background(), tC.log.Verifier.Name(), tC.wit.verifier.Name(), tC.log.checkpoint(tC.size, tC.hashSeed, tC.wit.signer))
			if (err != nil) != tC.wantErr {
				t.Errorf("unexpected error output (wantErr: %t): %v", tC.wantErr, err)
			}
		})
	}
}

func verifierOrDie(vkey string) note.Verifier {
	v, err := note.NewVerifier(vkey)
	if err != nil {
		panic(err)
	}
	return v
}

func signerOrDie(skey string) note.Signer {
	s, err := note.NewSigner(skey)
	if err != nil {
		panic(err)
	}
	return s
}

type fakeLog struct {
	distributor.LogInfo
	signer note.Signer
}

func (l fakeLog) checkpoint(size uint64, hashSeed string, wit note.Signer) []byte {
	hbs := sha256.Sum256([]byte(hashSeed))
	rawCP := log.Checkpoint{
		Origin: l.Origin,
		Size:   size,
		Hash:   hbs[:],
	}.Marshal()
	n := note.Note{}
	n.Text = string(rawCP)
	bs, err := note.Sign(&n, []note.Signer{l.signer, wit}...)
	if err != nil {
		panic(err)
	}
	return bs
}

type fakeWitness struct {
	verifier note.Verifier
	signer   note.Signer
}
