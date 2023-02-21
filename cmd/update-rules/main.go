/*
Copyright 2021 The Kubernetes Authors.

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
	"os"
	"path/filepath"

	"github.com/golang/glog"
	"gopkg.in/yaml.v2"

	"k8s.io/publishing-bot/cmd/publishing-bot/config"
)

const MainBranchName = "master"

type options struct {
	branch    string
	rulesFile string
	goVersion string
	out       string
}

func parseOptions() options {
	var o options
	flag.StringVar(&o.branch, "branch", "", "[required] Branch to update rules for, e.g. --branch release-x.yy")
	flag.StringVar(&o.rulesFile, "rules", "", "[required] URL or Path of the rules file to update rules for, e.g. --rules path/or/url/to/rules/file.yaml")
	flag.StringVar(&o.goVersion, "go", "", "Golang version to pin for this branch, e.g. --go 1.16.1")
	flag.StringVar(&o.out, "o", "", "Path to export the updated rules to, e.g. -o /tmp/rules.yaml")

	examples := `
  Examples:
  # Update rules for branch release-1.21 with go version 1.16.1
  update-rules -branch release-1.21 -go 1.16.4 -rules /go/src/k8s.io/kubernetes/staging/publishing/rules.yaml

  # Update rules using URL to input rules file
  update-rules -branch release-1.21 -go 1.16.4 -rules https://raw.githubusercontent.com/kubernetes/kubernetes/master/staging/publishing/rules.yaml

  # Update rules and export to /tmp/rules.yaml
  update-rules -branch release-1.22 -go 1.17.1 -o /tmp/rules.yaml -rules /go/src/k8s.io/kubernetes/staging/publishing/rules.yaml`

	flag.Usage = func() {
		fmt.Fprintf(os.Stdout, "\n  Usage:  update-rules --branch BRANCH --rules PATHorURL [--go VERSION | -o PATH]")
		fmt.Fprintf(os.Stdout, "\n  %s\n\n", examples)
		flag.PrintDefaults()
	}

	flag.Parse()

	if o.branch == "" {
		glog.Errorf("branch flag requires a non-empty value, e.g. --branch release-x.yy. Run `update-rules -h` for help!")
		os.Exit(2)
	}

	if o.rulesFile == "" {
		glog.Errorf("rules flag requires a non-empty value, e.g. --rules path/or/url/to/rules/file.yaml")
		os.Exit(2)
	}

	return o
}

func main() {
	o := parseOptions()

	// load and validate input rules file
	rules, err := load(o.rulesFile)
	if err != nil {
		glog.Fatal(err)
	}

	// update rules for all destination repos
	UpdateRules(rules, o.branch, o.goVersion)

	// validate rules after update
	if err := config.Validate(rules); err != nil {
		glog.Fatalf("update failed, found invalid rules after update: %v", err)
	}

	data, err := yaml.Marshal(rules)
	if err != nil {
		glog.Fatalf("error marshaling rules %v", err)
	}

	if o.out != "" {
		err = exportRules(o.out, data)
		if err != nil {
			glog.Fatalf("error exporting the rules %v", err)
		}
	} else {
		fmt.Fprintln(os.Stdout, string(data))
	}
}

// load reads the input rules file and validates the rules
func load(rulesFile string) (*config.RepositoryRules, error) {
	rules, err := config.LoadRules(rulesFile)
	if err != nil {
		return nil, fmt.Errorf("error loading rules file %q: %v", rulesFile, err)
	}

	if err := config.Validate(rules); err != nil {
		return nil, fmt.Errorf("invalid rules file %q: %v", rulesFile, err)
	}
	return rules, nil
}

func UpdateRules(rules *config.RepositoryRules, branch, goVer string) {
	// run the update per destination repo in the rules
	for j, r := range rules.Rules {
		var mainBranchRuleFound bool
		var newBranchRule config.BranchRule
		// find the mainBranch rules
		for _, br := range r.Branches {
			if br.Name == MainBranchName {
				cloneBranchRule(&br, &newBranchRule)
				mainBranchRuleFound = true
				break
			}
		}

		// if mainBranch rules not found for repo, it means it's removed from master tree, log warning and skip updating the rules
		if !mainBranchRuleFound {
			glog.Warningf("%s branch rules not found for repo %s, skipping to update branch %s rules", MainBranchName, r.DestinationRepository, branch)
			continue
		}

		// update the rules for branch and its dependencies
		updateBranchRules(&newBranchRule, branch, goVer)

		var branchRuleExists bool
		// if the target branch rules already exists, update it
		for i, br := range r.Branches {
			if br.Name == branch {
				glog.Infof("found branch %s rules for destination repo %s, updating it", branch, r.DestinationRepository)
				r.Branches[i] = newBranchRule
				branchRuleExists = true
				break
			}
		}
		// new rules, append to destination's branches
		if !branchRuleExists {
			r.Branches = append(r.Branches, newBranchRule)
		}

		// update the rules for destination repo
		rules.Rules[j] = r
	}
}

func cloneBranchRule(in, out *config.BranchRule) {
	if in == nil {
		return
	}
	*out = *in
	if in.Dependencies != nil {
		out.Dependencies = make([]config.Dependency, len(in.Dependencies))
		copy(out.Dependencies, in.Dependencies)
	}
}

func updateBranchRules(br *config.BranchRule, branch, goVersion string) {
	br.Name = branch
	br.Source.Branch = branch
	br.GoVersion = goVersion
	for k := range br.Dependencies {
		br.Dependencies[k].Branch = branch
	}
}

func exportRules(fPath string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(fPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(fPath, data, 0o644)
}
