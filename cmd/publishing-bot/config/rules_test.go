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

package config

import "testing"

func TestValidateGoVersion(t *testing.T) {
	tests := []struct {
		ver     string
		isValid bool
	}{
		{"0.0", true},
		{"0.0rc1", true},
		{"0.9", true},
		{"0.9.0", false},
		{"1.9", true},
		{"0.0.1", true},
		{"1.9.0", false},
		{"1.9.1", true},
		{"1.15", true},
		{"1.15.0", false},
		{"1.15.1", true},
		{"1.15beta1", true},
		{"1.15.0-beta.1", false},
		{"1.15rc1", true},
		{"1.15.0-rc.1", false},
		{"1.15.10", true},
		{"1.15.11", true},
		{"1.15.20", true},
		{"2.0", false},
		{"2.0alpha1", true},
		{"2.0-alpha1", false},
		{"12.12.100", true},
		{"999.999.9999", true},
		{"999.999.9999-beta.1", false},
		{"999.999.9999beta1", true},
		{"1.21.0", true},
		{"1.21", false},
		{"1.21rc1", true},
		{"1.21.0rc1", true},
		{"1.20.0", false},
		{"1.20rc1", true},
		{"1.22.0", true},
		{"1.22rc1", true},
		{"1.122.0", true},
		{"1.122", false},
		{"1.30.0", true},
		{"1.20rc.20rc2", false},
		{"2.20", false},
		{"2.21", false},
	}

	for _, test := range tests {
		err := ensureValidGoVersion(test.ver)
		if err != nil {
			// got error, but the version is valid
			if test.isValid {
				t.Errorf("go version check failed for valid version '%s''", test.ver)
			}
		} else {
			// got no error, but the version is invalid
			if !test.isValid {
				t.Errorf("go version '%s' is invalid, but got no error", test.ver)
			}
		}
	}
}
