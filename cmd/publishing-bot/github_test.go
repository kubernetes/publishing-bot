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
	"fmt"
	"testing"
)

func TestTail(t *testing.T) {
	tests := []struct {
		msg      string
		maxBytes int
		want     string
	}{
		{"", 10, ""},
		{"012", 10, "012"},
		{"0123456789", 10, "0123456789"},
		{"01234567890", 10, "...4567890"},
		{"\n01234567890", 10, "...4567890"},
		{"01234567890\n", 10, "...4567890"},
		{"01234\n0123", 10, "01234\n0123"},
		{"0123456\n0123", 10, "...\n0123"},
		{"0123\n0123\n0123", 10, "...\n0123"},
		{"01\n01\n01\n01", 10, "...\n01\n01"},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprintf("test %d", i), func(t *testing.T) {
			if got := tail(tt.msg, tt.maxBytes); got != tt.want {
				t.Errorf("tail(%q, %d) = %v, want %v", tt.msg, tt.maxBytes, got, tt.want)
			}
		})
	}
}
