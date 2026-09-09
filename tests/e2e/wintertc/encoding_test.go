package wintertc_test

import (
	"testing"
)

func TestLinuxAMD64WinterTCTypedArrays(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_typed_arrays",
		source: `
const buffer = new ArrayBuffer(4);
console.log(buffer.byteLength);
const bytes = new Uint8Array(buffer);
bytes[0] = 65;
bytes[1] = 511;
bytes[2] = 66;
bytes[3] = 67;
console.log(bytes.length);
console.log(bytes[0]);
console.log(bytes[1]);
const copy = bytes.slice(1, 3);
console.log(copy.length);
console.log(copy[0]);
const view = bytes.subarray(1, 3);
console.log(view.byteOffset);
console.log(view.length);
view[0] = 42;
console.log(bytes[1]);
console.log(copy[0]);
`,
		expected: "4\n4\n65\n255\n2\n255\n1\n2\n42\n255\n",
	})
}

func TestLinuxAMD64WinterTCTextEncoding(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_text_encoding",
		source: `
const encoder = new TextEncoder();
console.log(encoder.encoding);
const encoded = encoder.encode("hé✓");
console.log(encoded.length);
console.log(encoded[0]);
console.log(encoded[1]);
console.log(encoded[5]);
const decoder = new TextDecoder("  UTF8\t");
console.log(decoder.decode(encoded));
const malformed = new Uint8Array(2);
malformed[0] = 0xc0;
malformed[1] = 0xaf;
console.log(new TextDecoder().decode(malformed));
try {
  new TextDecoder("utf-8", { fatal: true }).decode(malformed);
  console.log("bad-fatal");
} catch (err) {
  console.log(err.name);
}
const bom = new Uint8Array(4);
bom[0] = 0xef; bom[1] = 0xbb; bom[2] = 0xbf; bom[3] = 65;
console.log(new TextDecoder().decode(bom));
const preserved = encoder.encode(new TextDecoder("unicode-1-1-utf-8", { ignoreBOM: true }).decode(bom));
console.log(preserved.length);
console.log(preserved[0]);
const destination = new Uint8Array(4);
const metrics = encoder.encodeInto("hé✓", destination);
console.log(metrics.read);
console.log(metrics.written);
console.log(destination[0]);
console.log(destination[3]);
try {
  new TextDecoder("windows-1252");
  console.log("bad-label");
} catch (err) {
  console.log(err.name);
}
`,
		expected: "utf-8\n6\n104\n195\n147\nhé✓\n��\nTypeError\nA\n4\n239\n2\n3\n104\n0\nRangeError\n",
	})
}

func TestLinuxAMD64WinterTCBodyJSONScalars(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_body_json_scalars",
		source: `
async function test(): Promise<void> {
  const req = new Request("http://example.com/", { method: "POST", body: "123.5" });
  console.log(await req.json());
  console.log(req.bodyUsed);
  try { await req.text(); console.log("reuse:unexpected"); } catch (err: any) { console.log("reuse:" + err.name); }

  const t = new Response("true");
  console.log(await t.json());
  console.log(t.bodyUsed);

  const f = new Response("false");
  console.log(await f.json());

  const n = new Response("null");
  console.log(await n.json());

  const locked = new Response("1");
  const lockedStream = locked.body ?? new ReadableStream();
  lockedStream.getReader();
  try { await locked.json(); console.log("locked:unexpected"); } catch (err: any) { console.log("locked:" + err.name); }
}
test();
`,
		expected: "123.5\ntrue\nreuse:TypeError\ntrue\ntrue\nfalse\nnull\nlocked:TypeError\n",
	})
}

func TestLinuxAMD64RuntimeJSONStructuredConformance(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "runtime_json_structured_conformance",
		source: `
function parseRuntime(text: string): any { return JSON.parse(text); }
function bad(text: string): void {
  try {
    JSON.parse(text);
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }
}
const obj: any = parseRuntime('{"a":1,"nested":{"x":2},"items":[10,20,30],"emoji":"\\uD83D\\uDE00","escaped":"a\\nb","e":1.5e2}');
console.log(obj.a);
console.log(obj.nested.x);
console.log(obj.items[1]);
console.log(obj.emoji);
console.log(obj.escaped);
console.log(obj.e);
const top: any = parseRuntime('[1,{"x":9}]');
console.log(top[1].x);
console.log(parseRuntime('1e-2'));
bad('[1,]');
bad('{"a":1,}');
bad('01');
bad('1e');
bad('true false');
bad('"\\x"');
bad('"unterminated');
`,
		expected: "1\n2\n20\n😀\na\nb\n150\n9\n0.01\nSyntaxError\nSyntaxError\nSyntaxError\nSyntaxError\nSyntaxError\nSyntaxError\nSyntaxError\n",
	})
}

func TestLinuxAMD64WinterTCBodyJSONStructured(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_body_json_structured",
		source: `
async function test(): Promise<void> {
  const response = new Response('{"name":"ts-pro","items":[1,{"x":7},3],"emoji":"\\uD83D\\uDE00","e":2.5e2}');
  const value: any = await response.json();
  console.log(value.name);
  console.log(value.items[1].x);
  console.log(value.emoji);
  console.log(value.e);
  console.log(response.bodyUsed);
  try {
    const bad = new Response('[1,]');
    await bad.json();
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }
}
test();
`,
		expected: "ts-pro\n7\n😀\n250\ntrue\nSyntaxError\n",
	})
}
