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
	"reflect"
	"testing"
)

var (
	testdataRules        = "testdata/rules.yaml"
	testdataInvalidRules = "testdata/invalid_rules.yaml"
	remoteRules          = "https://raw.githubusercontent.com/kubernetes/kubernetes/master/staging/publishing/rules.yaml"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expectErr bool
	}{
		{
			"local testdata valid rules file",
			testdataRules,
			false,
		},
		{
			"local testdata invalid rules file with invalid go version",
			testdataInvalidRules,
			true,
		},
		{
			"remote valid rules file",
			remoteRules,
			false,
		},
		{
			"local invalid path to rules file",
			"/invalid/path.yaml",
			true,
		},
		{
			"remote 404 rules files",
			"https://foo.bar/rules.yaml",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := load(tt.input)
			if err != nil && !tt.expectErr {
				t.Errorf("error loading test rules file from %s , did not expect error", tt.input)
			}
			if err == nil && tt.expectErr {
				t.Errorf("expected error while loading rules from %s , but got none", tt.input)
			}
		})
	}
}

func TestUpdateRules(t *testing.T) {
	tests := []struct {
		name      string
		branch    string
		goVersion string
	}{
		{
			"new branch with go version",
			"release-1.XY",
			"1.17.1",
		},
		{
			"new branch with go version",
			"release-1.XY",
			"1.17.1",
		},
		{
			"new branch without go version",
			"release-1.XY",
			"",
		},
		{
			"existing branch rule with go version update",
			"release-1.21",
			"1.16.1",
		},
		{
			"master branch rule update for go version",
			"master",
			"1.16.4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules, err := load(testdataRules)
			if err != nil {
				t.Errorf("error loading test rules file %v", err)
			}
			UpdateRules(rules, tt.branch, tt.goVersion, false)

			for _, repoRule := range rules.Rules {
				var masterRulePresent, branchRulePresent bool
				var masterRuleIndex, branchRuleIndex int

				for i, branchRule := range repoRule.Branches {
					switch branchRule.Name {
					case "master":
						masterRulePresent = true
						masterRuleIndex = i
					case tt.branch:
						branchRulePresent = true
						branchRuleIndex = i
					}
				}
				switch masterRulePresent {
				case true:
					if !branchRulePresent && tt.branch != "master" {
						t.Errorf("error updating branch %s rule for repo %s", tt.branch, repoRule.DestinationRepository)
					}
				case false:
					if branchRulePresent {
						t.Errorf("incorrectly added branch %s rule for repo %s whose master branch rule does not exists", tt.branch, repoRule.DestinationRepository)
					}
				}

				if repoRule.Branches[branchRuleIndex].Source.Branch != tt.branch {
					t.Errorf("incorrect update to branch %s rule for source branch field for repo %s", tt.branch, repoRule.DestinationRepository)
				}

				if repoRule.Branches[masterRuleIndex].Source.Dir != repoRule.Branches[branchRuleIndex].Source.Dir {
					t.Errorf("incorrect update to branch %s rule for source dir field for repo %s", tt.branch, repoRule.DestinationRepository)
				}

				if repoRule.Branches[branchRuleIndex].GoVersion != tt.goVersion {
					t.Errorf("incorrect go version set for branch %s rule for repo %s", tt.branch, repoRule.DestinationRepository)
				}

				if len(repoRule.Branches[masterRuleIndex].Dependencies) != len(repoRule.Branches[branchRuleIndex].Dependencies) {
					t.Errorf("incorrect update to branch %s rule dependencies for repo %s", tt.branch, repoRule.DestinationRepository)
				}

				if len(repoRule.Branches[masterRuleIndex].RequiredPackages) != len(repoRule.Branches[branchRuleIndex].RequiredPackages) {
					t.Errorf("incorrect update to branch %s rule required packages for repo %s", tt.branch, repoRule.DestinationRepository)
				}

				if repoRule.Branches[masterRuleIndex].SmokeTest != repoRule.Branches[branchRuleIndex].SmokeTest {
					t.Errorf("incorrect update to branch %s rule smoke-test for repo %s", tt.branch, repoRule.DestinationRepository)
				}
			}
		})
	}
}

func TestDeleteRules(t *testing.T) {
	tests := []struct {
		name          string
		branch        string
		goVersion     string
		isBranchExist bool
	}{
		{
			"deleting rule for non existing branch",
			"release-1.20",
			"1.17.1",
			true,
		},
		{
			"deleting rule for non existing branch 1.25",
			"release-1.25",
			"1.17.1",
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules, err := load(testdataRules)
			if err != nil {
				t.Errorf("error loading test rules file %v", err)
			}
			UpdateRules(rules, tt.branch, tt.goVersion, true)
			if tt.isBranchExist {
				for _, repoRule := range rules.Rules {
					for _, branchRule := range repoRule.Branches {
						if branchRule.Name == tt.branch {
							t.Errorf("failed to delete %s branch rule from for repo %s", tt.name, repoRule.DestinationRepository)
						}
					}
				}
			} else {
				if loadedRules, err := load(testdataRules); err != nil {
					t.Errorf("error loading test rules file for comparison %v", err)
				} else if !reflect.DeepEqual(loadedRules, rules) {
					t.Errorf("rules changed after deleting a non existent branch %s", tt.branch)
				}
			}
		})
	}
}
