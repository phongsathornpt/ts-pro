package wintertc_test

import (
	"testing"

	"github.com/phongsathornpt/ts-pro/tests/e2e/harness"
)

type linuxAMD64Case struct {
	name     string
	source   string
	expected string
}

func runLinuxAMD64(t *testing.T, tc linuxAMD64Case) {
	t.Helper()
	t.Parallel()
	harness.RunLinuxAMD64(t, harness.LinuxAMD64Case{
		Name:     tc.name,
		Source:   tc.source,
		Expected: tc.expected,
	})
}

func mustReadExample(t *testing.T, path string) string {
	t.Helper()
	return harness.MustReadExample(t, path)
}
