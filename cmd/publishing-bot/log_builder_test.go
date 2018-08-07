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
		builder := newLogBuilderWithMaxBytes(c.maxBytes, c.rawLog)
		if l := builder.log(); l != c.expectedLog {
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
fooworld
foobaz
foobarfoo`
	actual := newLogBuilderWithMaxBytes(0, testLog).
		addHeading("****").
		trim("\n").
		split("\n").
		filter(func(line string) bool {
			return strings.HasPrefix(line, "foo")
		}).
		tail(3).
		join("\n").
		log()
	if expected != actual {
		t.Errorf("log mismatched: expected(%q) actual(%q)", expected, actual)
	}
}
