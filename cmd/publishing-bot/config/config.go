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

package config

// Config is how we are configured to talk to github.
type Config struct {
	// the organization to publish into, e.g. k8s-publishing-bot or kubernetes-nightly
	TargetOrg string `yaml:"target-org"`

	// the source repo name, e.g. "kubernetes"
	SourceRepo string `yaml:"source-repo"`

	// the source repo org name, e.g. "kubernetes"
	SourceOrg string `yaml:"source-org"`

	// the file with the clear-text github token
	TokenFile string `yaml:"token-file,omitempty"`

	// the file that contain the repository rules
	RulesFile string `yaml:"rules-file"`

	// comma separated list of source repository branches we will read the rule file from
	RuleSourceBranches string `yaml:"rule-source-branches"`

	// If true, don't make any mutating API calls
	DryRun bool

	// A github issue number to report errors
	GithubIssue int `yaml:"github-issue,omitempty"`

	// BasePublishScriptPath determine the base path where we will look for a
	// publishing scripts in the source repo. It defaults to ./publishing_scripts'.
	BasePublishScriptPath string `yaml:"base-publish-script-path,omitempty"`
}
