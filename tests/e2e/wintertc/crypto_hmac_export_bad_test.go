package wintertc_test

import (
	"testing"

	"github.com/phongsathornpt/ts-pro/tests/e2e/harness"
)

func TestSubtleCryptoHMACExportRejectsInvalidKeyType(t *testing.T) {
	harness.RunBad(t, harness.BadCase{
		Name:         "subtle_crypto_hmac_export_invalid_key",
		Source:       `crypto.subtle.exportKey("raw", "not-a-key");`,
		ExpectedCode: "TS2345",
		ExpectedSub:  `crypto.subtle.exportKey argument "key"`,
	})
}
