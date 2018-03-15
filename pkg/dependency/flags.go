/*
Copyright 2018 The Kubernetes Authors.

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

package dependency

import (
	"strings"
)

// ParseDependencies parse a comma separated string of repo:branch pairs.
func ParseDependencies(s string) ([]Dependency, error) {
	var dependentRepos []Dependency
	if len(s) > 0 {
		for _, pair := range strings.Split(s, ",") {
			ps := strings.Split(pair, ":")
			d := Dependency{
				Name: ps[0],
			}
			if len(ps) >= 2 {
				d.Branch = ps[1]
			}
			dependentRepos = append(dependentRepos, d)
		}
	}

	return dependentRepos, nil
}
