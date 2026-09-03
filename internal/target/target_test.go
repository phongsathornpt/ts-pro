package target

import "testing"

func TestParseSupportedTargets(t *testing.T) {
	for _, tc := range []struct {
		os   string
		arch string
	}{
		{os: "linux", arch: "amd64"},
		{os: "darwin", arch: "amd64"},
		{os: "darwin", arch: "arm64"},
	} {
		if _, err := Parse(tc.os, tc.arch); err != nil {
			t.Fatalf("Parse(%q, %q): %v", tc.os, tc.arch, err)
		}
	}
}

func TestParseRejectsUnsupportedTargets(t *testing.T) {
	for _, tc := range []struct {
		os   string
		arch string
	}{
		{os: "linux", arch: "arm64"},
		{os: "linux", arch: "potato"},
		{os: "windows", arch: "amd64"},
	} {
		if _, err := Parse(tc.os, tc.arch); err == nil {
			t.Fatalf("Parse(%q, %q) unexpectedly succeeded", tc.os, tc.arch)
		}
	}
}
