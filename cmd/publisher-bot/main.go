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

	"k8s.io/publishing-bot/cmd/publisher-bot/config"
)

func Usage() {
	fmt.Fprintf(os.Stderr, `
Usage: %s [-config <config-yaml-file>] [-dry-run] [-token-file <token-file>]
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
	sourceRepo := flag.String("source-repo", "", `the source repo (defaults to "kubernetes")`)
	targetOrg := flag.String("target-org", "", `the target organization to publish into (e.g. "k8s-publishing-bot")`)

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

	// override with flags
	if *dryRun {
		cfg.DryRun = true
	}
	if *targetOrg != "" {
		cfg.TargetOrg = *targetOrg
	}
	if *sourceRepo != "" {
		cfg.SourceRepo = *sourceRepo
	}
	if *tokenFile != "" {
		cfg.TokenFile = *tokenFile
	}

	// default
	if cfg.SourceRepo == "" {
		cfg.SourceRepo = "kubernetes"
	}

	publisher, err := New(&cfg)
	if err != nil {
		glog.Fatalf("Failed initialize publisher: %v", err)
	}

	if err := publisher.Run(); err != nil {
		glog.Fatalf("Failed to run publisher: %v", err)
	}
}
