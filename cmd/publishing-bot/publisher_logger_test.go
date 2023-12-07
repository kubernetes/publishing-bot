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

package main

import (
	"bytes"
	"sync"
	"testing"
)

func TestLogLineWriter(t *testing.T) {
	buf := new(bytes.Buffer)
	fakeLogWriter := newSyncWriter(
		muxWriter{buf},
	)

	content1 := "XXXXXXXXXXXXXX"
	content2 := "YYYYYYYYYYYYYY"
	content3 := "ZZZZZZZZZZZZZZ"

	contents := []string{
		content1, content2, content3,
	}

	wg := &sync.WaitGroup{}
	for _, content := range contents {
		w := newLineWriter(fakeLogWriter)
		content := content
		wg.Add(1)
		go func() {
			for i := 0; i < 99999; i++ {
				//nolint:errcheck  // TODO(lint): Should we be checking errors here?
				w.Write([]byte(content + "\n"))
			}
			wg.Done()
		}()
	}
	wg.Wait()

	finalContent := buf.String()
	uniqueLines := make(map[string]struct{})

	newLogBuilderWithMaxBytes(0, finalContent).
		Trim("\n").
		Split("\n").
		Filter(func(line string) bool {
			uniqueLines[line] = struct{}{}
			return true
		}).Log()

	for line := range uniqueLines {
		if line != content1 && line != content2 && line != content3 {
			t.Errorf("malformed log: %s", line)
			t.Fail()
		}
	}
}
