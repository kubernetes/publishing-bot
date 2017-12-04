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

import (
	"io/ioutil"
	"strings"
)

// Config is how we are configured to talk to github.
type Config struct {
	// the organization to publish into, e.g. k8s-publishing-bot or kubernetes-nightly
	TargetOrg string `yaml:"target-org"`

	// the source repo, e.g. "kubernetes"
	// TODO: make this absolute
	SourceRepo string `yaml:"source-repo"`

	// the file with the clear-text github token
	TokenFile string `yaml:"token-file"`
	token     string

	// If true, don't make any mutating API calls
	DryRun bool
}

// Token returns the token.
func (config *Config) Token() (string, error) {
	if config.token == "" {
		bs, err := ioutil.ReadFile(config.TokenFile)
		if err != nil {
			return "", err
		}
		config.token = strings.Trim(string(bs), " \t\n")
	}
	return config.token, nil
}
