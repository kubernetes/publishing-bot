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

// Changing glog output directory via --log_dir doesn't work, because the flag
// is parsed after the first invocation of glog, so the log file ends up in the
// temporary directory. Hence, we manually duplicates glog ouptut.

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/shurcooL/go/indentwriter"
	"gopkg.in/natefinch/lumberjack.v2"
)

type plog struct {
	combinedBufAndFile io.Writer
	buf                *bytes.Buffer
}

func newPublisherLog(buf *bytes.Buffer, logFileName string) (*plog, error) {
	logFile := &lumberjack.Logger{
		Filename: logFileName,
		MaxAge:   7,
	}
	if err := logFile.Rotate(); err != nil {
		return nil, err
	}

	return &plog{newSyncWriter(muxWriter{buf, logFile}), buf}, nil
}

func (p *plog) write(s string) {
	//nolint:errcheck  // TODO(lint): Should we be checking errors here?
	p.combinedBufAndFile.Write([]byte("[" + time.Now().Format(time.RFC822) + "]: "))

	//nolint:errcheck  // TODO(lint): Should we be checking errors here?
	p.combinedBufAndFile.Write([]byte(s))

	//nolint:errcheck  // TODO(lint): Should we be checking errors here?
	p.combinedBufAndFile.Write([]byte("\n"))
}

func (p *plog) Errorf(format string, args ...interface{}) {
	s := prefixFollowingLines("    ", fmt.Sprintf(format, args...))
	glog.ErrorDepth(1, s)
	p.write(s)
}

func (p *plog) Infof(format string, args ...interface{}) {
	s := prefixFollowingLines("    ", fmt.Sprintf(format, args...))
	glog.InfoDepth(1, s)
	p.write(s)
}

func (p *plog) Fatalf(format string, args ...interface{}) {
	s := prefixFollowingLines("    ", fmt.Sprintf(format, args...))
	glog.FatalDepth(1, s)
	p.write(s)
}

func (p *plog) Run(c *exec.Cmd) error {
	p.Infof("%s", cmdStr(c))

	errBuf := &bytes.Buffer{}

	stdoutLineWriter := newLineWriter(muxWriter{p.combinedBufAndFile, os.Stdout})
	stderrLineWriter := newLineWriter(muxWriter{p.combinedBufAndFile, errBuf})
	c.Stdout = indentwriter.New(stdoutLineWriter, 1)
	c.Stderr = indentwriter.New(stderrLineWriter, 1)

	err := c.Start()
	if err != nil {
		p.Errorf("failed to start %q: %v", c.Path, err)
		return err
	}
	err = c.Wait()
	if err != nil {
		p.Errorf("%s\n%s", err.Error(), errBuf.String())
	}
	stdoutLineWriter.Flush()
	stderrLineWriter.Flush()
	return err
}

func (p *plog) Logs() string {
	return p.buf.String()
}

func (p *plog) Flush() {
	glog.Flush()
}

func prefixFollowingLines(p, s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		if i != 0 && lines[i] != "" {
			lines[i] = p + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

func cmdStr(cs *exec.Cmd) string {
	args := make([]string, len(cs.Args))
	for i, s := range cs.Args {
		if strings.ContainsRune(s, ' ') {
			args[i] = fmt.Sprintf("%q", s)
		} else {
			args[i] = s
		}
	}
	return strings.Join(args, " ")
}

type muxWriter []io.Writer

func (mw muxWriter) Write(b []byte) (int, error) {
	n := 0
	var err error
	for _, w := range mw {
		if n, err = w.Write(b); err != nil {
			return n, err
		}
	}
	return n, nil
}

func newLineWriter(writer io.Writer) lineWriter {
	return lineWriter{
		buf:    new(bytes.Buffer),
		writer: writer,
	}
}

type lineWriter struct {
	buf    *bytes.Buffer
	writer io.Writer
}

func (lw lineWriter) Write(b []byte) (int, error) {
	n := 0
	for idx := range b {
		if err := lw.buf.WriteByte(b[idx]); err != nil {
			return n, err
		}
		if b[idx] == '\n' {
			if _, err := lw.Flush(); err != nil {
				return n, err
			}
		}
		n++
	}
	return n, nil
}

//nolint:unparam  // TODO(lint): result 0 (int) is never used
func (lw lineWriter) Flush() (int, error) {
	written, err := lw.buf.WriteTo(lw.writer)
	lw.buf.Reset()
	return int(written), err
}

func newSyncWriter(writer io.Writer) syncWriter {
	return syncWriter{
		writer: writer,
		lock:   &sync.Mutex{},
	}
}

type syncWriter struct {
	writer io.Writer
	lock   *sync.Mutex
}

func (sw syncWriter) Write(b []byte) (int, error) {
	sw.lock.Lock()
	defer sw.lock.Unlock()
	return sw.writer.Write(b)
}
