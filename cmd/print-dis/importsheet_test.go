package main

import (
	"io"
	"testing"
)

func TestParseImportArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want importArgs
		ok   bool
	}{
		{"file first", []string{"a.csv", "--requester", "x@y.z", "--dry-run"}, importArgs{"a.csv", "x@y.z", true}, true},
		{"file last", []string{"--requester=x@y.z", "a.csv"}, importArgs{"a.csv", "x@y.z", false}, true},
		{"file between", []string{"--dry-run", "a.csv", "--requester", "x@y.z"}, importArgs{"a.csv", "x@y.z", true}, true},
		{"no file", []string{"--requester", "x@y.z"}, importArgs{}, false},
		{"two files", []string{"a.csv", "b.csv", "--requester", "x@y.z"}, importArgs{}, false},
		{"no requester", []string{"a.csv"}, importArgs{}, false},
		{"bad flag", []string{"a.csv", "--nope"}, importArgs{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseImportArgs(tc.args, io.Discard)
			if (err == nil) != tc.ok {
				t.Fatalf("err = %v", err)
			}
			if tc.ok && *got != tc.want {
				t.Errorf("got %+v want %+v", *got, tc.want)
			}
		})
	}
}
