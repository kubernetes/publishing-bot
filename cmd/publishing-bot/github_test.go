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

import "testing"

func TestGithubLogTransform(t *testing.T) {
	//nolint:dupword // this is intentional for the test
	originLog := `111111111111
222222
+ hello
hello foo
hello bar
+ holla
holla foo
holla bar
+ hi
hi foo
hi bar
`
	//nolint:dupword // this is intentional for the test
	expected := "```" + `
+ hello
hello foo
hello bar
+ holla
holla foo
holla bar
+ hi
hi foo
hi bar` + "```\n"
	actual := transfromLogToGithubFormat(originLog, 3)
	if actual != expected {
		t.Errorf("log mismatched: expected(%q) actual(%q)", expected, actual)
		t.Fail()
	}
}
