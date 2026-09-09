package wintertc_test

import (
	"testing"
)

func TestLinuxAMD64WinterTCBlobCore(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_blob_core",
		source: `
async function test(): Promise<void> {
  const b1 = new Blob();
  console.log(b1.size);
  console.log(b1.type);

  const b2 = new Blob(["hello ", "world"], { type: "TEXT/PLAIN; charset=utf-8" });
  console.log(b2.size);
  console.log(b2.type);
  const text2 = await b2.text();
  console.log(text2);

  const s1 = b2.slice(0, 5);
  console.log(s1.size);
  const textS1 = await s1.text();
  console.log(textS1);

  const s2 = b2.slice(-5);
  console.log(s2.size);
  const textS2 = await s2.text();
  console.log(textS2);

  const s3 = b2.slice(0, 5, "IMAGE/PNG");
  console.log(s3.type);

  const ab = await b2.arrayBuffer();
  console.log(ab.byteLength);

  const bytes = await b2.bytes();
  console.log(bytes.length);
  console.log(bytes[0]);
  console.log(bytes[10]);
}
test();
`,
		expected: "0\n\n11\ntext/plain; charset=utf-8\nhello world\n5\nhello\n5\nworld\nimage/png\n11\n11\n104\n100\n",
	})
}

func TestLinuxAMD64WinterTCFileCore(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_file_core",
		source: `
async function test(): Promise<void> {
  const f1 = new File(["file-data"], "test.txt");
  console.log(f1.name);
  console.log(f1.size);
  console.log(f1.webkitRelativePath);
  console.log(f1.lastModified > 0);

  const f2 = new File(["content"], "custom.bin", { type: "APPLICATION/OCTET-STREAM", lastModified: 999999 });
  console.log(f2.name);
  console.log(f2.type);
  console.log(f2.lastModified);
  const textF2 = await f2.text();
  console.log(textF2);
}
test();
`,
		expected: "test.txt\n9\n\ntrue\ncustom.bin\napplication/octet-stream\n999999\ncontent\n",
	})
}

func TestLinuxAMD64WinterTCBlobFileFormDataConformance(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_blob_file_formdata_conformance",
		source:   mustReadExample(t, "../../examples/wintertc/blob_file_formdata_conformance.ts"),
		expected: "0\nempty-type\n11\ntext/plain; charset=utf-8\nhello world\n5\nhello\n5\nworld\nimage/png\n11\n11\n104\n100\ntest.txt\n9\ntrue\ncustom.bin\napplication/octet-stream\n999999\ncontent\n1\nnull-val\ntrue\nfalse\n2\n1\n3\n1\n100\nfalse\nblob\nblobby\ncustom.dat\ncustom.bin\nrenamed.bin\nx:10\ny:20\nx:30\n",
	})
}

func TestLinuxAMD64WinterTCBodyFormDataURLEncoded(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_body_form_data_urlencoded",
		source: `
async function test(): Promise<void> {
  const req = new Request("http://example.com/", {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded; charset=UTF-8" },
    body: "a=1&b=hello+world&a=2&encoded=%E0%B8%81",
  });
  const reqForm = await req.formData();
  console.log(reqForm.get("a"));
  console.log(reqForm.getAll("a").length);
  console.log(reqForm.getAll("a")[1]);
  console.log(reqForm.get("b"));
  console.log(reqForm.get("encoded"));
  console.log(req.bodyUsed);

  const res = new Response("x=10&y=a%2Bb", {
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
  });
  const resForm = await res.formData();
  console.log(resForm.get("x"));
  console.log(resForm.get("y"));
  console.log(res.bodyUsed);

  try {
    const bad = new Response("x=1", { headers: { "Content-Type": "text/plain" } });
    await bad.formData();
    console.log("bad:unexpected");
  } catch (err: any) {
    console.log("bad:" + err.name);
  }
}
test();
`,
		expected: "1\n2\n2\nhello world\nก\ntrue\n10\na+b\ntrue\nbad:TypeError\n",
	})
}

func TestLinuxAMD64PromiseResolveNonThenableFormData(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "promise_resolve_non_thenable_formdata",
		source: `
async function test(): Promise<void> {
  const direct = new FormData();
  direct.append("z", "9");
  const promised = await Promise.resolve(direct);
  console.log(promised.get("z"));
}
test();
`,
		expected: "9\n",
	})
}

func TestLinuxAMD64WinterTCBodyFormDataMultipart(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_body_formdata_multipart",
		source: `
async function test(): Promise<void> {
  const prefixText = "--AaB03x\r\n" +
    "Content-Disposition: form-data; name=\"field\"\r\n\r\n" +
    "first\r\n" +
    "--AaB03x\r\n" +
    "Content-Disposition: form-data; name=\"field\"\r\n\r\n" +
    "second\r\n" +
    "--AaB03x\r\n" +
    "Content-Disposition: form-data; name=\"upload\"; filename=\"data.bin\"\r\n" +
    "Content-Type: application/octet-stream\r\n\r\n" +
    "A";
  const suffixText = "B\r\n--AaB03x--\r\n";
  const encoder = new TextEncoder();
  const prefix = encoder.encode(prefixText);
  const suffix = encoder.encode(suffixText);
  const body = new Uint8Array(prefix.length + 1 + suffix.length);
  let i = 0;
  for (let j = 0; j < prefix.length; j += 1) { body[i] = prefix[j]; i += 1; }
  body[i] = 0; i += 1;
  for (let j = 0; j < suffix.length; j += 1) { body[i] = suffix[j]; i += 1; }
  const res = new Response(body, { headers: { "Content-Type": "multipart/form-data; boundary=\"AaB03x\"" } });
  const form = await res.formData();
  const fields = form.getAll("field");
  console.log(fields.length);
  console.log(fields[0]);
  console.log(fields[1]);
  const file: File = form.get("upload");
  console.log(file.name);
  console.log(file.type);
  console.log(file.size);
  const bytes = await file.bytes();
  console.log(bytes[0]);
  console.log(bytes[1]);
  console.log(bytes[2]);
  console.log(res.bodyUsed);
}
test();
`,
		expected: "2\nfirst\nsecond\ndata.bin\napplication/octet-stream\n3\n65\n0\n66\ntrue\n",
	})
}

func TestLinuxAMD64WinterTCBodyFormDataMultipartValidation(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_body_formdata_multipart_validation",
		source: `
async function test(): Promise<void> {
  const body = "--simple\r\nContent-Disposition: form-data; name=\"a\"\r\n\r\n1\r\n--simple--\r\n";
  const ok = new Request("http://example.com/", { method: "POST", body: body, headers: { "Content-Type": "multipart/form-data; boundary=simple" } });
  const form = await ok.formData();
  console.log(form.get("a"));
  console.log(ok.bodyUsed);

  const missing = new Response(body, { headers: { "Content-Type": "multipart/form-data" } });
  try { await missing.formData(); console.log("missing:unexpected"); } catch (err: any) { console.log("missing:" + err.name); }

  const malformed = new Response("--x\r\nContent-Disposition: form-data; name=\"a\"\r\n\r\n1", { headers: { "Content-Type": "multipart/form-data; boundary=x" } });
  try { await malformed.formData(); console.log("malformed:unexpected"); } catch (err: any) { console.log("malformed:" + err.name); }
}
test();
`,
		expected: "1\ntrue\nmissing:TypeError\nmalformed:TypeError\n",
	})
}
