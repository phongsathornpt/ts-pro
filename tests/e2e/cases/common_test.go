package cases_test

import (
	"testing"

	"github.com/phongsathornpt/ts-pro/tests/e2e/harness"
)

type happyCase struct {
	name     string
	source   string
	expected string
}

type badCase struct {
	name         string
	source       string
	expectedCode string
	expectedSub  string
}

func runHappy(t *testing.T, tc happyCase) {
	t.Helper()
	t.Parallel()
	harness.RunHappy(t, harness.HappyCase{
		Name:     tc.name,
		Source:   tc.source,
		Expected: tc.expected,
	})
}

func runBad(t *testing.T, tc badCase) {
	t.Helper()
	t.Parallel()
	harness.RunBad(t, harness.BadCase{
		Name:         tc.name,
		Source:       tc.source,
		ExpectedCode: tc.expectedCode,
		ExpectedSub:  tc.expectedSub,
	})
}
