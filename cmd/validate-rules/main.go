/*
Copyright 2019 The Kubernetes Authors.

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

	"k8s.io/publishing-bot/cmd/publishing-bot/config"
)

func main() {
	flag.Parse()

	for _, f := range flag.Args() {
		rules, err := config.LoadRules(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Cannot load rules file %q: %v\n", f, err)
			os.Exit(1)
		}
		if err := config.Validate(rules); err != nil {
			fmt.Fprintf(os.Stderr, "Invalid rules file %q: %v\n", f, err)
			os.Exit(1)
		}
	}
}
