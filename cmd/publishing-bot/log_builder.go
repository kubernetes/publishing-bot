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

func newLogBuilderWithMaxBytes(maxBytes int, rawLogs ...string) *logBuilder {
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

func (builder *logBuilder) addHeading(line string) *logBuilder {
	builder.headings = append(builder.headings, line)
	return builder
}

func (builder *logBuilder) addTailing(line string) *logBuilder {
	builder.tailings = append(builder.tailings, line)
	return builder
}

func (builder *logBuilder) split(sep string) *logBuilder {
	var splittedLogs []string
	for _, log := range builder.logs {
		splittedLogs = append(splittedLogs, strings.Split(log, sep)...)
	}
	builder.logs = splittedLogs
	return builder
}

func (builder *logBuilder) trim(cutset string) *logBuilder {
	for idx := range builder.logs {
		builder.logs[idx] = strings.Trim(builder.logs[idx], cutset)
	}
	return builder
}

func (builder *logBuilder) filter(predicate func(line string) bool) *logBuilder {
	var filteredLogs []string
	for i := 0; i < len(builder.logs); i++ {
		if predicate(builder.logs[i]) {
			filteredLogs = append(filteredLogs, builder.logs[i])
		}
	}
	builder.logs = filteredLogs
	return builder
}

func (builder *logBuilder) tail(n int) *logBuilder {
	if len(builder.logs) <= n {
		return builder
	}
	builder.logs = builder.logs[len(builder.logs)-n:]
	return builder
}

func (builder *logBuilder) join(sep string) *logBuilder {
	builder.logs = []string{strings.Join(builder.logs, sep)}
	return builder
}

func (builder *logBuilder) log() string {
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
