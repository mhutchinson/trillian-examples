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

// distributor is a server designed to make witnessed checkpoints of
// verifiable logs available to clients in an efficient manner.
package main

import (
	"context"
	"database/sql"
	"flag"
	"net"
	"net/http"

	"github.com/golang/glog"
	"github.com/google/trillian-examples/distributor/cmd/internal/distributor"
	ihttp "github.com/google/trillian-examples/distributor/cmd/internal/http"
	"github.com/gorilla/mux"
	"golang.org/x/mod/sumdb/note"
	"golang.org/x/sync/errgroup"
)

var (
	addr   = flag.String("listen", ":8080", "Address to listen on")
	dbFile = flag.String("db_file", "", "Path to database file")
)

func main() {
	flag.Parse()
	ctx := context.Background()

	// This error group will be used to run all top level processes.
	// If any process dies, then all of them will be stopped via context cancellation.
	g, ctx := errgroup.WithContext(ctx)
	httpListener, err := net.Listen("tcp", *addr)
	if err != nil {
		glog.Fatalf("failed to listen on %q", *addr)
	}

	// TODO(mhutchinson): These need to be initialized
	ws := make(map[string]note.Verifier)
	ls := make(map[string]distributor.LogInfo)
	// TODO(mhutchinson): This should be a non-sqlite DB
	if len(*dbFile) == 0 {
		glog.Fatalf("db_file is required")
	}
	// Start up local database.
	glog.Infof("Connecting to local DB at %q", *dbFile)
	db, err := sql.Open("sqlite3", *dbFile)
	if err != nil {
		glog.Fatalf("Failed to connect to DB: %v", err)
	}
	// Avoid "database locked" issues with multiple concurrent updates.
	db.SetMaxOpenConns(1)

	d := distributor.NewDistributor(ws, ls, db)
	r := mux.NewRouter()
	s := ihttp.NewServer(d)
	s.RegisterHandlers(r)
	srv := http.Server{
		Handler: r,
	}
	g.Go(func() error {
		glog.Info("HTTP server goroutine started")
		defer glog.Info("HTTP server goroutine done")
		return srv.Serve(httpListener)
	})
	g.Go(func() error {
		// This goroutine brings down the HTTP server when ctx is done.
		glog.Info("HTTP server-shutdown goroutine started")
		defer glog.Info("HTTP server-shutdown goroutine done")
		<-ctx.Done()
		return srv.Shutdown(ctx)
	})
	if err := g.Wait(); err != nil {
		glog.Errorf("failed with error: %v", err)
	}
}
