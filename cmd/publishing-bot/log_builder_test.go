/*
Copyright 2016 The Kubernetes Authors.

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
	"strings"
	"testing"
)

func TestNewGithubLogBuilder(t *testing.T) {
	testCases := []struct {
		rawLog      string
		maxBytes    int
		expectedLog string
	}{
		{
			rawLog:      "abcdefg",
			maxBytes:    5,
			expectedLog: "...fg",
		},
		{
			rawLog:      "abcdefg",
			maxBytes:    1000,
			expectedLog: "abcdefg",
		},
		{
			rawLog:      "abcdefg",
			maxBytes:    -1,
			expectedLog: "abcdefg",
		},
		{
			rawLog:      "",
			maxBytes:    100,
			expectedLog: "",
		},
	}
	for _, c := range testCases {
		builder := NewLogBuilderWithMaxBytes(c.maxBytes, c.rawLog)
		if l := builder.Log(); l != c.expectedLog {
			t.Errorf("log mismatched: expected(%q) actual(%q)", c.expectedLog, l)
			t.Fail()
		}
	}
}

func TestGithubLogBuilderManipulation(t *testing.T) {
	testLog := `foohello
fooworld
foobaz
foobarfoo
barbarbar
`
	expected := `****
foobarfoo
foobaz
fooworld`
	actual := NewLogBuilderWithMaxBytes(0, testLog).
		AddHeading("****").
		Trim("\n").
		Split("\n").
		Filter(func(line string) bool {
			return strings.HasPrefix(line, "foo")
		}).
		Tail(3).
		Reverse().
		Join("\n").
		Log()
	if expected != actual {
		t.Errorf("log mismatched: expected(%q) actual(%q)", expected, actual)
	}
}
