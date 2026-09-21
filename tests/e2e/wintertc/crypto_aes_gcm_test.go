package wintertc_test

import "testing"

func TestSubtleCryptoAESGCM128KnownAnswerAndRoundTrip(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "subtle_crypto_aes_gcm_128",
		source: `
async function main(): Promise<void> {
  const keyBytes = new Uint8Array(16);
  const iv = new Uint8Array(12);
  const aad = new Uint8Array(0);
  const plain = new Uint8Array(16);

  const key = await crypto.subtle.importKey("raw", keyBytes, { name: "aEs-GcM" }, true, ["encrypt", "decrypt"]);
  console.log(key.type);
  console.log(key.extractable);
  console.log(key.algorithm.name);
  console.log(key.algorithm.length);
  console.log(key.usages.length);

  const params = { name: "AES-GCM", iv: iv, additionalData: aad, tagLength: 128 };
  const encrypted = new Uint8Array(await crypto.subtle.encrypt(params, key, plain));
  console.log(encrypted.length);
  for (let i = 0; i < encrypted.length; i = i + 1) console.log(encrypted[i]);

  const decrypted = new Uint8Array(await crypto.subtle.decrypt(params, key, encrypted));
  console.log(decrypted.length);
  let allZero = true;
  for (let i = 0; i < decrypted.length; i = i + 1) {
    if (decrypted[i] !== 0) allZero = false;
  }
  console.log(allZero);
}
main();
`,
		expected: "secret\ntrue\nAES-GCM\n128\n2\n32\n3\n136\n218\n206\n96\n182\n163\n146\n243\n40\n194\n185\n113\n178\n254\n120\n171\n110\n71\n212\n44\n236\n19\n189\n245\n58\n103\n178\n18\n87\n189\n223\n16\ntrue\n",
	})
}

func TestSubtleCryptoAESGCMRejections(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "subtle_crypto_aes_gcm_rejections",
		source: `
async function main(): Promise<void> {
  const iv = new Uint8Array(12);
  const aad = new Uint8Array(0);
  const plain = new Uint8Array(16);

  try {
    await crypto.subtle.importKey("raw", new Uint8Array(24), { name: "AES-GCM" }, false, ["encrypt"]);
    console.log("unexpected");
  } catch (err: any) { console.log(err.name); }

  try {
    await crypto.subtle.importKey("raw", new Uint8Array(16), { name: "AES-GCM" }, false, ["sign"]);
    console.log("unexpected");
  } catch (err: any) { console.log(err.name); }

  const encryptOnly = await crypto.subtle.importKey("raw", new Uint8Array(16), { name: "AES-GCM" }, false, ["encrypt"]);
  const params = { name: "AES-GCM", iv: iv, additionalData: aad, tagLength: 128 };

  try {
    await crypto.subtle.decrypt(params, encryptOnly, new Uint8Array(32));
    console.log("unexpected");
  } catch (err: any) { console.log(err.name); }

  try {
    await crypto.subtle.encrypt({ name: "AES-GCM", iv: new Uint8Array(8), additionalData: aad, tagLength: 128 }, encryptOnly, plain);
    console.log("unexpected");
  } catch (err: any) { console.log(err.name); }

  try {
    await crypto.subtle.encrypt({ name: "AES-GCM", iv: iv, additionalData: aad, tagLength: 96 }, encryptOnly, plain);
    console.log("unexpected");
  } catch (err: any) { console.log(err.name); }
}
main();
`,
		expected: "DataError\nSyntaxError\nInvalidAccessError\nOperationError\nOperationError\n",
	})
}
