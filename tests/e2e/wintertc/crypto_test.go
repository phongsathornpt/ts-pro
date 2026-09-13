package wintertc_test

import (
	"strings"
	"testing"

	"github.com/phongsathornpt/ts-pro/tests/e2e/harness"
)

func TestCryptoGetRandomValues(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "crypto_get_random_values",
		source: `const backing = new Uint8Array(36);
backing[0] = 11;
backing[1] = 22;
backing[34] = 33;
backing[35] = 44;

const view = backing.subarray(2, 34);
const same = crypto.getRandomValues(view);
console.log(same === view);
console.log(backing[0]);
console.log(backing[1]);
console.log(backing[34]);
console.log(backing[35]);

let changed = false;
for (let i = 2; i < 34; i = i + 1) {
  if (backing[i] !== 0) {
    changed = true;
  }
}
console.log(changed);

const maximum = new Uint8Array(65536);
console.log(crypto.getRandomValues(maximum).length);
console.log(crypto.getRandomValues(new Uint8Array(0)).length);

try {
  crypto.getRandomValues(new Uint8Array(65537));
  console.log("unexpected");
} catch (err: any) {
  console.log(err.name);
}
`,
		expected: "true\n11\n22\n33\n44\ntrue\n65536\n0\nQuotaExceededError\n",
	})
}

func TestCryptoRandomUUID(t *testing.T) {
	out, executed := harness.RunLinuxAMD64Output(t, "crypto_random_uuid", `console.log(crypto.randomUUID());
console.log(crypto.randomUUID());
`)
	if !executed {
		return
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("randomUUID stdout lines: got %d, want 2: %q", len(lines), out)
	}
	for _, uuid := range lines {
		if !isCanonicalUUIDV4(uuid) {
			t.Fatalf("randomUUID returned non-canonical UUID v4: %q", uuid)
		}
	}
}

func TestCryptoRandomUUIDRejectsArguments(t *testing.T) {
	harness.RunBad(t, harness.BadCase{
		Name:         "crypto_random_uuid_arguments",
		Source:       `crypto.randomUUID(1);`,
		ExpectedCode: "TS2554",
		ExpectedSub:  "crypto.randomUUID expects no arguments",
	})
}

func TestSubtleCryptoDigestSHA256(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "subtle_crypto_digest_sha256",
		source: `async function main(): Promise<void> {
  const backing = new Uint8Array(5);
  backing[0] = 9;
  backing[1] = 97;
  backing[2] = 98;
  backing[3] = 99;
  backing[4] = 7;

  const digest = await crypto.subtle.digest("SHA-256", backing.subarray(1, 4));
  const bytes = new Uint8Array(digest);
  console.log(bytes.length);
  for (let i = 0; i < bytes.length; i = i + 1) {
    console.log(bytes[i]);
  }

  const encoded = new TextEncoder().encode("abc");
  const lower = new Uint8Array(await crypto.subtle.digest("sha-256", encoded.buffer));
  console.log(lower[0]);
  console.log(lower[31]);

  const mixed = new Uint8Array(await crypto.subtle.digest("sHa-256", encoded));
  console.log(mixed[0]);
  console.log(mixed[31]);

  const dictionary = new Uint8Array(await crypto.subtle.digest({ name: "ShA-256" }, encoded));
  console.log(dictionary[0]);
  console.log(dictionary[31]);

  try {
    await crypto.subtle.digest("MD5", encoded);
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }
}
main();
`,
		expected: "32\n186\n120\n22\n191\n143\n1\n207\n234\n65\n65\n64\n222\n93\n174\n34\n35\n176\n3\n97\n163\n150\n23\n122\n156\n180\n16\n255\n97\n242\n0\n21\n173\n186\n173\n186\n173\n186\n173\nNotSupportedError\n",
	})
}

func TestSubtleCryptoDigestSHA384AndSHA512(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "subtle_crypto_digest_sha384_sha512",
		source: `async function main(): Promise<void> {
  const encoded = new TextEncoder().encode("abc");

  const sha384 = new Uint8Array(await crypto.subtle.digest("sHa-384", encoded));
  console.log(sha384.length);
  console.log(sha384[0]);
  console.log(sha384[1]);
  console.log(sha384[46]);
  console.log(sha384[47]);

  const sha512 = new Uint8Array(await crypto.subtle.digest({ name: "ShA-512" }, encoded.buffer));
  console.log(sha512.length);
  console.log(sha512[0]);
  console.log(sha512[1]);
  console.log(sha512[62]);
  console.log(sha512[63]);
}
main();
`,
		expected: "48\n203\n0\n37\n167\n64\n221\n175\n164\n159\n",
	})
}

func TestSubtleCryptoDigestRejectsInvalidDataType(t *testing.T) {
	harness.RunBad(t, harness.BadCase{
		Name:         "subtle_crypto_digest_invalid_data",
		Source:       `crypto.subtle.digest("SHA-256", "abc");`,
		ExpectedCode: "TS2345",
		ExpectedSub:  "crypto.subtle.digest expects ArrayBuffer or Uint8Array data",
	})
}

func isCanonicalUUIDV4(uuid string) bool {
	if len(uuid) != 36 {
		return false
	}
	for _, index := range []int{8, 13, 18, 23} {
		if uuid[index] != '-' {
			return false
		}
	}
	if uuid[14] != '4' {
		return false
	}
	if !strings.ContainsRune("89ab", rune(uuid[19])) {
		return false
	}
	for i := 0; i < len(uuid); i++ {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		if !strings.ContainsRune("0123456789abcdef", rune(uuid[i])) {
			return false
		}
	}
	return true
}
