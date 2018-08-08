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
	"bytes"
	"strings"
)

type logBuilder struct {
	logs     []string
	headings []string
	tailings []string
}

func NewLogBuilderWithMaxBytes(maxBytes int, rawLogs ...string) *logBuilder {
	ignoreBytesLimits := maxBytes <= 0
	size := 0
	logBuilder := &logBuilder{}
	for i := len(rawLogs) - 1; i >= 0; i-- {
		if curSize := size + len(rawLogs[i]); !ignoreBytesLimits && curSize > maxBytes {
			rawLogs[i] = rawLogs[i][curSize-maxBytes:]
			rawLogs[i] = "..." + string(rawLogs[i][3:])
			logBuilder.logs = append(logBuilder.logs, rawLogs[i])
			break
		}
		logBuilder.logs = append(logBuilder.logs, rawLogs[i])
		size += len(rawLogs)
	}
	return logBuilder
}

func (builder *logBuilder) AddHeading(lines... string) *logBuilder {
	builder.headings = append(builder.headings, lines...)
	return builder
}

func (builder *logBuilder) AddTailing(lines... string) *logBuilder {
	builder.tailings = append(builder.tailings, lines...)
	return builder
}

func (builder *logBuilder) Split(sep string) *logBuilder {
	var splittedLogs []string
	for _, log := range builder.logs {
		splittedLogs = append(splittedLogs, strings.Split(log, sep)...)
	}
	builder.logs = splittedLogs
	return builder
}

func (builder *logBuilder) Trim(cutset string) *logBuilder {
	for idx := range builder.logs {
		builder.logs[idx] = strings.Trim(builder.logs[idx], cutset)
	}
	return builder
}

func (builder *logBuilder) Reverse() *logBuilder {
	s := builder.logs
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
	return builder
}

func (builder *logBuilder) Filter(predicate func(line string) bool) *logBuilder {
	var filteredLogs []string
	for i := 0; i < len(builder.logs); i++ {
		if predicate(builder.logs[i]) {
			filteredLogs = append(filteredLogs, builder.logs[i])
		}
	}
	builder.logs = filteredLogs
	return builder
}

func (builder *logBuilder) Tail(n int) *logBuilder {
	if len(builder.logs) <= n {
		return builder
	}
	builder.logs = builder.logs[len(builder.logs)-n:]
	return builder
}

func (builder *logBuilder) Join(sep string) *logBuilder {
	builder.logs = []string{strings.Join(builder.logs, sep)}
	return builder
}

func (builder *logBuilder) Log() string {
	buf := new(bytes.Buffer)
	for _, heading := range builder.headings {
		buf.WriteString(heading + "\n")
	}
	for _, log := range builder.logs {
		buf.WriteString(log)
	}
	for _, tailing := range builder.tailings {
		buf.WriteString(tailing + "\n")
	}
	return buf.String()
}
