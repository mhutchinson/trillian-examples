// Copyright 2021 Google LLC. All Rights Reserved.
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

// feeder/github redistributes checkpoints from a witness into a serverless
// distributor by raising a PR containing the co-signed checkpoint.
package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"time"

	"github.com/golang/glog"
	"gopkg.in/yaml.v2"

	"github.com/google/trillian-examples/serverless/config"
	"github.com/google/trillian-examples/serverless/internal/github"
	"golang.org/x/mod/sumdb/note"

	dist_gh "github.com/google/trillian-examples/serverless/internal/distribute/github"
	wit_http "github.com/google/trillian-examples/witness/golang/client/http"
)

const usage = `Usage:
 distribute --distributor_repo --fork_repo --distributor_path --config_file --interval

Where:
 --distributor_repo is the repo owner/fragment from the distributor github repo URL.
     e.g. github.com/AlCutter/serverless-test -> AlCutter/serverless-test
 --distributor_branch is the name of the primary branch on the distributor repo (e.g. main).
 --fork_repo is the repo owner/fragment of the forked distributor to use for the PR branch.
 --distributor_path is the path from the root of the repo where the distributor files can be found,
 --config_file is the path to the config file for the serverless/cmd/distribute/github command.
 --interval if set, the script will continuously feed and (if needed) create witness PRs sleeping
     the specified number of seconds between attempts. If not provided, the tool does a one-shot feed.

`

var (
	distributorRepo   = flag.String("distributor_repo", "", "The repo owner/fragment from the distributor repo URL.")
	distributorBranch = flag.String("distributor_branch", "master", "The branch that PRs will be proposed against on the distributor_repo.")
	forkRepo          = flag.String("fork_repo", "", "The repo owner/fragment from the feeder (forked distributor) repo URL.")
	distributorPath   = flag.String("distributor_path", "", "Path from the root of the repo where the distributor files can be found.")
	configPath        = flag.String("config_file", "", "Path to the config file for the serverless/cmd/distribute/github command.")
	configB64         = flag.String("config_b64", "", "Configuration yaml file encoded as b64, as an alternative to config_file.")
	interval          = flag.Duration("interval", time.Duration(0), "Interval between checkpoints. Default of 0 causes the tool to be a one-shot.")
)

func main() {
	flag.Parse()
	ctx := context.Background()

	opts := mustConfigure(ctx)

	if err := dist_gh.DistributeOnce(ctx, opts); err != nil {
		glog.Warningf("DistributeOnce: %v", err)
	}
	if *interval > 0 {
		for {
			select {
			case <-time.After(*interval):
				glog.V(1).Infof("Wait time is up, going around again (%s)", *interval)
			case <-ctx.Done():
				return
			}
			if err := dist_gh.DistributeOnce(ctx, opts); err != nil {
				glog.Warningf("DistributeOnce: %v", err)
			}
		}
	}
}

// usageExit prints out a message followed by the usage string, and then terminates execution.
func usageExit(m string) {
	glog.Exitf("%s\n\n%s", m, usage)
}

// mustConfigure creates an options struct from flags and env vars.
// It will terminate execution on any error.
func mustConfigure(ctx context.Context) *dist_gh.DistributeOptions {
	checkNotEmpty := func(m, v string) {
		if v == "" {
			usageExit(m)
		}
	}
	// Check flags
	checkNotEmpty("Missing required --distributor_repo flag", *distributorRepo)
	checkNotEmpty("Missing required --fork_repo flag", *forkRepo)
	checkNotEmpty("Missing required --distributor_path flag", *distributorPath)
	checkNotEmpty("Missing required --config_file flag", *configPath)

	// Check env vars
	githubAuthToken := os.Getenv("GITHUB_AUTH_TOKEN")
	gitUsername := os.Getenv("GIT_USERNAME")
	gitEmail := os.Getenv("GIT_EMAIL")

	checkNotEmpty("Unauthorized: No GITHUB_AUTH_TOKEN present", githubAuthToken)
	checkNotEmpty("Environment variable GIT_USERNAME is required to make commits", gitUsername)
	checkNotEmpty("Environment variable GIT_EMAIL is required to make commits", gitEmail)

	dr, err := github.NewRepoID(*distributorRepo)
	if err != nil {
		usageExit(fmt.Sprintf("--distributor_repo invalid: %v", err))
	}
	fr, err := github.NewRepoID(*forkRepo)
	if err != nil {
		usageExit(fmt.Sprintf("--fork_repo invalid: %v", err))
	}

	if len(*configPath) == 0 && len(*configB64) == 0 {
		usageExit("Missing required --config_path OR --config_b64 flags")
	}
	var cfg distributeConfig
	if len(*configPath) > 0 {
		cfg, err = readConfig(*configPath)
		if err != nil {
			glog.Exitf("Feeder config in %q is invalid: %v", *configPath, err)
		}
	} else {
		bs, err := base64.RawStdEncoding.DecodeString(*configB64)
		if err != nil {
			glog.Exitf("Failed to decode --config_b64 string")
		}
		cfg, err = unmarshalConfig(bs)
		if err != nil {
			glog.Exitf("Feeder config in --config_b64 is invalid: %v", *configPath, err)
		}
	}

	u, err := url.Parse(cfg.Witness.URL)
	if err != nil {
		glog.Exitf("Failed to parse witness URL %q: %v", cfg.Witness.URL, err)
	}
	wSigV, err := note.NewVerifier(cfg.Witness.PublicKey)
	if err != nil {
		glog.Exitf("Invalid witness public key for url %q: %v", cfg.Witness.URL, err)
	}

	lSigV, err := note.NewVerifier(cfg.Log.PublicKey)
	if err != nil {
		glog.Exitf("Invalid log public key: %v", err)
	}

	repo, err := github.NewRepository(ctx, dr, *distributorBranch, fr, gitUsername, gitEmail, githubAuthToken)
	if err != nil {
		glog.Exitf("Failed to set up repository: %v", err)
	}

	return &dist_gh.DistributeOptions{
		Repo:            repo,
		DistributorPath: *distributorPath,
		Log:             cfg.Log,
		LogSigV:         lSigV,
		WitSigV:         wSigV,
		Witness: wit_http.Witness{
			URL:      u,
			Verifier: wSigV,
		},
	}
}

// distributeConfig is the format of this tool's config file.
type distributeConfig struct {
	// Log defines the log checkpoints are being distributed for.
	Log config.Log `yaml:"Log"`

	// Witness defines the witness to read from.
	Witness config.Witness `yaml:"Witness"`
}

// Validate checks that the config is populated correctly.
func (c distributeConfig) Validate() error {
	if err := c.Log.Validate(); err != nil {
		return err
	}
	return c.Witness.Validate()
}

// readConfig parses the named file into a distributeConfig structure.
func readConfig(f string) (distributeConfig, error) {
	c, err := ioutil.ReadFile(f)
	if err != nil {
		return distributeConfig{}, fmt.Errorf("failed to read file: %v", err)
	}
	return unmarshalConfig(c)
}

func unmarshalConfig(bs []byte) (distributeConfig, error) {
	cfg := distributeConfig{}
	if err := yaml.Unmarshal(bs, &cfg); err != nil {
		return cfg, fmt.Errorf("failed to unmarshal config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		return cfg, fmt.Errorf("invalid config: %v", err)
	}
	return cfg, nil
}
