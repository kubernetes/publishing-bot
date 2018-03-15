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

package golang

import (
	"os"
	"path/filepath"
	"testing"
)

func Test_fullPackageName(t *testing.T) {
	gopath := os.Getenv("GOPATH")
	cwd, _ := os.Getwd()
	tests := []struct {
		dir     string
		want    string
		wantErr bool
	}{
		{"", cwd[len(gopath)+5:], false},
		{"/foo", "", true},
		{filepath.Join(gopath, "foo"), "", true},
		{filepath.Join(gopath, "src/foo"), "foo", false},
		{filepath.Join(gopath, "src/foo/bar"), "foo/bar", false},
		{"../foo", filepath.Join(filepath.Dir(cwd), "foo")[len(gopath)+5:], false},
	}
	for _, tt := range tests {
		got, err := FullPackageName(tt.dir)
		if (err != nil) != tt.wantErr {
			t.Errorf("fullPackageName(%q) = %q, %v; wantErr %v", tt.dir, got, err, tt.wantErr)
			return
		}
		if got != tt.want {
			t.Errorf("fullPackageName(%q) = %v, %v; want %v", tt.dir, got, err, tt.want)
		}
	}
}
