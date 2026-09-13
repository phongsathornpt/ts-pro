package wintertc_test

import (
	"strings"
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
	if winterTCParallelSafe(t.Name()) {
		t.Parallel()
	}
	harness.RunLinuxAMD64(t, harness.LinuxAMD64Case{
		Name:     tc.name,
		Source:   tc.source,
		Expected: tc.expected,
	})
}

func winterTCParallelSafe(testName string) bool {
	if strings.HasPrefix(testName, "TestLinuxAMD64WinterTCURL") ||
		strings.HasPrefix(testName, "TestLinuxAMD64WinterTCBlob") {
		return true
	}

	switch testName {
	case "TestCryptoGetRandomValues",
		"TestLinuxAMD64WinterTCTypedArrays",
		"TestLinuxAMD64WinterTCTextEncoding",
		"TestLinuxAMD64WinterTCBodyJSONScalars",
		"TestLinuxAMD64RuntimeJSONStructuredConformance",
		"TestLinuxAMD64WinterTCBodyJSONStructured",
		"TestLinuxAMD64WinterTCQueuingStrategies",
		"TestLinuxAMD64WinterTCReadableStreamCore",
		"TestLinuxAMD64WinterTCWritableStreamCore",
		"TestLinuxAMD64WinterTCTransformStreamPipe",
		"TestLinuxAMD64WinterTCStreamsConformance",
		"TestLinuxAMD64WinterTCBodyStreamBacking",
		"TestLinuxAMD64WinterTCBodyStreamDisturbance",
		"TestLinuxAMD64WinterTCBodyLockedIsUnusable":
		return true
	default:
		return false
	}
}

func mustReadExample(t *testing.T, path string) string {
	t.Helper()
	return harness.MustReadExample(t, path)
}
