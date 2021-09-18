// Copyright (c) 2021 Terminus, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"path/filepath"
	"testing"

	"k8s.io/publishing-bot/cmd/publishing-bot/config"
)

func Test_PublishErdaProto(t *testing.T) {
	flag.Parse()
	publisher := PublisherMunger{
		config: &config.Config{
			BasePublishScriptPath: "/Users/hoozecn/terminus.io/publishing-bot/artifacts/scripts",
			SourceRepo:            "erda",
			SourceOrg:             "hoozecn",
			TargetOrg:             "hoozecn",
			GithubHost:            "github.com",
			BasePackage:           "erda-project",
			TokenFile:             "/tmp/github-push-token",
		},
		reposRules: config.RepositoryRules{
			Rules: []config.RepositoryRule{
				{
					DestinationRepository: "erda-proto",
					Branches: []config.BranchRule{
						{
							Name: "master",
							Source: config.Source{
								Branch: "master",
								Dir:    "api/proto",
							},
						},
						//{
						//	Name: "release/1.3",
						//	Source: config.Source{
						//		Branch: "release/1.3",
						//		Dir:    "proto",
						//	},
						//},
					},
				},
				{
					DestinationRepository: "erda-proto-go",
					Branches: []config.BranchRule{
						{
							Name: "master",
							Source: config.Source{
								Branch: "master",
								Dir:    "api/proto-go",
							},
						},
						//{
						//	Name: "release/1.3",
						//	Source: config.Source{
						//		Branch: "release/1.3",
						//		Dir:    "proto-go",
						//	},
						//},
						//{
						//	Name: "release/1.2",
						//	Source: config.Source{
						//		Branch: "release/1.2",
						//		Dir:    "proto-go",
						//	},
						//},
					},
				},
			},
		},
		baseRepoPath: filepath.Join("/Users/hoozecn", "go", "src", "github.com", "hoozecn"),
	}

	publisher.Run()
}
