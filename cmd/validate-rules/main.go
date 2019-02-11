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
	"github.com/golang/glog"
	"k8s.io/publishing-bot/cmd/publishing-bot/config"
)

func main() {
	flag.Parse()
	flag.Set("alsologtostderr", "true")

	for _, f := range flag.Args() {
		rules, err := config.LoadRules(f)
		if err != nil {
			glog.Fatalf("Cannot load rules file %q: %v", f, err)
		}
		if err := config.Validate(rules); err != nil {
			glog.Fatalf("Invalid rules file %q: %v", f, err)
		}
		glog.Infof("validation successful")
	}
}
