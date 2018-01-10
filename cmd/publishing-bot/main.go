/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/golang/glog"
	"gopkg.in/yaml.v2"

	"strings"

	"time"

	"k8s.io/publishing-bot/cmd/publishing-bot/config"
)

func Usage() {
	fmt.Fprintf(os.Stderr, `
Usage: %s [-config <config-yaml-file>] [-dry-run] [-token-file <token-file>] [-interval <sec>]
          [-source-repo <repo>] [-target-org <org>]

Command line flags override config values.
`, os.Args[0])
	flag.PrintDefaults()
}

func main() {
	configFilePath := flag.String("config", "", "the config file in yaml format")
	dryRun := flag.Bool("dry-run", false, "do not push anything to github")
	tokenFile := flag.String("token-file", "", "the file with the github toke")
	// TODO: make absolute
	repoName := flag.String("repo-name", "", "the name of the source repository (eg. kubernetes)")
	repoOrg := flag.String("repo-org", "", "the name of the source repository organization, (eg. kubernetes)")
	targetOrg := flag.String("target-org", "", `the target organization to publish into (e.g. "k8s-publishing-bot")`)
	interval := flag.Uint("interval", 0, "loop with the given seconds of wait in between")
	healthzPort := flag.Int("healthz-port", 0, "start healthz webserver on the given port listening on 0.0.0.0")

	flag.Usage = Usage
	flag.Parse()

	cfg := config.Config{}
	if *configFilePath != "" {
		bs, err := ioutil.ReadFile(*configFilePath)
		if err != nil {
			glog.Fatalf("Failed to load config file from %q: %v", *configFilePath, err)
		}
		if err := yaml.Unmarshal(bs, &cfg); err != nil {
			glog.Fatalf("Failed to parse config file at %q: %v", *configFilePath, err)
		}
	}

	if len(*repoName) == 0 || len(*repoOrg) == 0 {
		glog.Fatalf("repo-org and repo-name cannot be empty")
	}

	// override with flags
	if *dryRun {
		cfg.DryRun = true
	}
	if *targetOrg != "" {
		cfg.TargetOrg = *targetOrg
	}
	if *repoName != "" {
		cfg.SourceRepoName = *repoName
	}
	if *repoOrg != "" {
		cfg.SourceRepoOrg = *repoOrg
	}
	if *tokenFile != "" {
		cfg.TokenFile = *tokenFile
	}

	if len(cfg.TargetOrg) == 0 {
		glog.Fatalf("Target organization cannot be empty")
	}

	// start healthz server
	healthz := Healthz{
		Issue:  cfg.GithubIssue,
		config: cfg,
	}
	if *healthzPort != 0 {
		if err := healthz.Run(*healthzPort); err != nil {
			glog.Fatalf("Failed to run healthz server: %v", err)
		}
	}

	for {
		last := time.Now()

		publisher, err := New(&cfg)
		if err != nil {
			glog.Fatalf("Failed initialize publisher: %v", err)
		}

		if cfg.TokenFile != "" && cfg.GithubIssue != 0 && !cfg.DryRun {
			// load token
			bs, err := ioutil.ReadFile(cfg.TokenFile)
			if err != nil {
				glog.Fatalf("Failed to load token file from %q: %v", cfg.TokenFile, err)
			}
			token := strings.Trim(string(bs), " \t\n")

			// run
			logs, hash, err := publisher.Run()
			healthz.SetHealth(err == nil, hash)
			if err != nil {
				glog.Infof("Failed to run publisher: %v", err)
				if err := ReportOnIssue(err, logs, token, cfg.TargetOrg, cfg.SourceRepoName, cfg.GithubIssue); err != nil {
					glog.Fatalf("Failed to report logs on github issue: %v", err)
				}
			} else if err := CloseIssue(token, cfg.TargetOrg, cfg.SourceRepoName, cfg.GithubIssue); err != nil {
				glog.Fatalf("Failed to close issue: %v", err)
			}
		} else {
			// run
			_, hash, err := publisher.Run()
			healthz.SetHealth(err == nil, hash)
			if err != nil {
				glog.Infof("Failed to run publisher: %v", err)
			}
		}

		if *interval == 0 {
			break
		}

		timeToSleep := time.Duration(int(*interval)-int(time.Since(last).Seconds())) * time.Second
		glog.Infof("Sleeping %v until next run", timeToSleep)
		time.Sleep(timeToSleep)
	}
}
