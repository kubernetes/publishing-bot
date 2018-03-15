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
	"os"
	"strings"

	"github.com/golang/glog"
	"k8s.io/publishing-bot/pkg/dependency"
	"k8s.io/publishing-bot/pkg/dependency/dep"
)

func Usage() {
	fmt.Fprintf(os.Stderr, `Convert Godeps/Godeps.json into Gopkg.toml.

Usage: %s --source-remote <remote> --source-branch <source-branch>
          [--dependencies <repo-name>[:branch]]
          [--required <pkg>,...]
          [--alternative-source <pkg>]
`, os.Args[0])
	flag.PrintDefaults()
}

func main() {
	dependencies := flag.String("dependencies", "", "comma-separated list of repo:branch pairs of dependencies")
	required := flag.String("required", "", "comma-separated list of Golang packages that are required")
	alternativeSource := flag.String("alternative-source", "", "a package org like github.com/sttts to be used as alternative source of the dependencies")

	flag.Usage = Usage
	flag.Parse()

	var requiredPkgs []string
	if len(*required) > 0 {
		requiredPkgs = strings.Split(*required, ",")
	}

	dependentRepos, err := dependency.ParseDependencies(*dependencies)
	if err != nil {
		glog.Fatalf("Failed to parse dependencies %q: %v", *dependencies, err)
	}

	if err := dep.GodepToGopkg(dependentRepos, requiredPkgs, *alternativeSource); err != nil {
		glog.Fatal(err)
	}
}
