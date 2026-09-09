package wintertc_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLinuxAMD64WinterTCHeadersConformance(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_headers_conformance",
		source:   mustReadExample(t, "../../examples/wintertc/headers_conformance.ts"),
		expected: "one, three\ntrue\nnull\n2\ncookie=a=1\ncookie=b=2\nset-cookie=a=1\nset-cookie=b=2\nx-a=one, three\nx-b=two\nafter=replacement\nhasb=false\n[set-cookie:a=1][set-cookie:b=2][x-a:replacement]\ncopy=replacement\nrec=x-y=y\nrec=x-z=z\nname=TypeError\nvalue=TypeError\n",
	})
}

func TestLinuxAMD64WinterTCRequestCore(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_request_core",
		source: `
async function test(): Promise<void> {
  const req = new Request("https://EXAMPLE.com/a?x=1", {
    method: "post",
    headers: { "X-Test": "  one  ", "Content-Type": "text/plain" },
    body: "hello request"
  });
  console.log(req.method);
  console.log(req.url);
  console.log(req.headers.get("x-test"));
  console.log(req.headers.get("content-type"));
  console.log(req.bodyUsed);

  const clone = req.clone();
  clone.headers.set("x-test", "two");
  console.log(req.headers.get("x-test"));
  console.log(clone.headers.get("x-test"));
  console.log(clone.method);
  console.log(clone.url);

  const text = await req.text();
  console.log(text);
  console.log(req.bodyUsed);

  const bytes = await clone.bytes();
  console.log(bytes.length);
  console.log(bytes[0]);
  console.log(clone.bodyUsed);

  const copied = new Request(clone, { method: "put" });
  console.log(copied.method);
  console.log(copied.url);
  console.log(copied.headers.get("x-test"));
}
test();
`,
		expected: "POST\nhttps://example.com/a?x=1\none\ntext/plain\nfalse\none\ntwo\nPOST\nhttps://example.com/a?x=1\nhello request\ntrue\n13\n104\ntrue\nPUT\nhttps://example.com/a?x=1\ntwo\n",
	})
}

func TestLinuxAMD64WinterTCResponseCore(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_response_core",
		source: `
async function test(): Promise<void> {
  const res = new Response("hello response", {
    status: 201,
    statusText: "Created",
    headers: { "X-Test": " one " }
  });
  console.log(res.status);
  console.log(res.statusText);
  console.log(res.ok);
  console.log(res.type);
  console.log(res.url);
  console.log(res.redirected);
  console.log(res.headers.get("x-test"));
  console.log(res.bodyUsed);

  const clone = res.clone();
  clone.headers.set("x-test", "two");
  console.log(res.headers.get("x-test"));
  console.log(clone.headers.get("x-test"));
  console.log(await res.text());
  console.log(res.bodyUsed);
  const bytes = await clone.bytes();
  console.log(bytes.length);
  console.log(bytes[0]);

  const redir = Response.redirect("https://example.com/next", 307);
  console.log(redir.status);
  console.log(redir.headers.get("location"));
  console.log(redir.ok);

  const err = Response.error();
  console.log(err.status);
  console.log(err.type);
  console.log(err.ok);

  const json = Response.json({ answer: 42 }, { status: 202 });
  console.log(json.status);
  console.log(json.headers.get("content-type"));
  console.log(await json.text());
}
test();
`,
		expected: "201\nCreated\ntrue\ndefault\n\nfalse\none\nfalse\none\ntwo\nhello response\ntrue\n14\n104\n307\nhttps://example.com/next\nfalse\n0\nerror\nfalse\n202\napplication/json\n{\"answer\":42}\n",
	})
}

func TestLinuxAMD64WinterTCResponseJSONInit(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_response_json_init",
		source: `
const a = Response.json({ answer: 42 }, {
  status: 201,
  statusText: "Created",
  headers: { "X-Test": "yes" },
});
console.log(a.status);
console.log(a.statusText);
console.log(a.headers.get("x-test"));
console.log(a.headers.get("content-type"));
console.log(await a.text());

const b = Response.json({ error: true }, {
  headers: { "Content-Type": "application/problem+json" },
});
console.log(b.headers.get("content-type"));
console.log(await b.text());

try { Response.json({}, { status: 199 }); console.log("bad-status:unexpected"); } catch (err: any) { console.log("bad-status:" + err.name); }
try { Response.json({}, { statusText: "bad\r\ntext" }); console.log("bad-text:unexpected"); } catch (err: any) { console.log("bad-text:" + err.name); }
try { Response.json({}, { status: 204 }); console.log("null-status:unexpected"); } catch (err: any) { console.log("null-status:" + err.name); }
`,
		expected: "201\nCreated\nyes\napplication/json\n{\"answer\":42}\napplication/problem+json\n{\"error\":true}\nbad-status:RangeError\nbad-text:TypeError\nnull-status:TypeError\n",
	})
}

func TestLinuxAMD64WinterTCRequestResponseValidation(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_request_response_validation",
		source: `
try { new Request("http://example.com/", { method: "GET", body: "x" }); console.log("get-body:unexpected"); } catch (err: any) { console.log("get-body:" + err.name); }
try { new Request("http://example.com/", { method: "HEAD", body: "x" }); console.log("head-body:unexpected"); } catch (err: any) { console.log("head-body:" + err.name); }
try { new Response(null, { status: 199 }); console.log("status-low:unexpected"); } catch (err: any) { console.log("status-low:" + err.name); }
try { new Response(null, { status: 600 }); console.log("status-high:unexpected"); } catch (err: any) { console.log("status-high:" + err.name); }
try { new Response(null, { statusText: "bad\r\ntext" }); console.log("status-text:unexpected"); } catch (err: any) { console.log("status-text:" + err.name); }
try { new Response("x", { status: 204 }); console.log("body-204:unexpected"); } catch (err: any) { console.log("body-204:" + err.name); }
try { new Response("x", { status: 205 }); console.log("body-205:unexpected"); } catch (err: any) { console.log("body-205:" + err.name); }
try { new Response("x", { status: 304 }); console.log("body-304:unexpected"); } catch (err: any) { console.log("body-304:" + err.name); }
try { Response.redirect("http://example.com/", 200); console.log("redirect-status:unexpected"); } catch (err: any) { console.log("redirect-status:" + err.name); }
const ok = new Response("x", { status: 201, statusText: "Created" });
console.log(ok.status);
console.log(ok.statusText);
console.log(Response.redirect("http://example.com/x", 307).status);
`,
		expected: "get-body:TypeError\nhead-body:TypeError\nstatus-low:RangeError\nstatus-high:RangeError\nstatus-text:TypeError\nbody-204:TypeError\nbody-205:TypeError\nbody-304:TypeError\nredirect-status:RangeError\n201\nCreated\n307\n",
	})
}

func TestLinuxAMD64WinterTCRequestDynamicMethodNormalization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, r.Method)
	}))
	defer server.Close()

	source := fmt.Sprintf(`
async function test(): Promise<void> {
  let standard: string = "post";
  const req = new Request(%q, { method: standard, body: "x" });
  console.log(req.method);
  let custom: string = "patch";
  const customReq = new Request(%q, { method: custom, body: "x" });
  console.log(customReq.method);
  let fetchMethod: string = "put";
  const res = await fetch(%q, { method: fetchMethod, body: "x" });
  console.log(await res.text());
}
test();
`, server.URL+"/request", server.URL+"/custom", server.URL+"/fetch")
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_request_dynamic_method_normalization",
		source:   source,
		expected: "POST\npatch\nPUT\n",
	})
}

func TestLinuxAMD64WinterTCRequestMethodValidation(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_request_method_validation",
		source: `
try { new Request("http://example.com/", { method: "CONNECT" }); console.log("connect:unexpected"); } catch (err: any) { console.log("connect:" + err.name); }
let traceMethod: string = "trace";
try { new Request("http://example.com/", { method: traceMethod }); console.log("trace:unexpected"); } catch (err: any) { console.log("trace:" + err.name); }
let trackMethod: string = "TrAcK";
try { new Request("http://example.com/", { method: trackMethod }); console.log("track:unexpected"); } catch (err: any) { console.log("track:" + err.name); }
let invalidMethod: string = "BAD METHOD";
try { new Request("http://example.com/", { method: invalidMethod }); console.log("token:unexpected"); } catch (err: any) { console.log("token:" + err.name); }
let custom: string = "patch";
console.log(new Request("http://example.com/", { method: custom }).method);
`,
		expected: "connect:TypeError\ntrace:TypeError\ntrack:TypeError\ntoken:TypeError\npatch\n",
	})
}

func TestLinuxAMD64WinterTCResponseRedirectURLValidation(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_response_redirect_url_validation",
		source: `
const res = Response.redirect("HTTP://EXAMPLE.COM:80/a/../b?x=1", 302);
console.log(res.status);
console.log(res.headers.get("location"));
const v6 = Response.redirect("http://[::1]:80/x", 307);
console.log(v6.headers.get("location"));
try { Response.redirect("/relative", 302); console.log("relative:unexpected"); } catch (err: any) { console.log("relative:" + err.name); }
try { Response.redirect("not a url", 302); console.log("invalid:unexpected"); } catch (err: any) { console.log("invalid:" + err.name); }
`,
		expected: "302\nhttp://example.com/b?x=1\nhttp://[::1]/x\nrelative:TypeError\ninvalid:TypeError\n",
	})
}

func TestLinuxAMD64WinterTCNullBodyInit(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_null_body_init",
		source: `
async function test(): Promise<void> {
  const res = new Response(null, { status: 204 });
  console.log(res.body === null);
  console.log(res.bodyUsed);
  console.log(await res.text());
  console.log(res.bodyUsed);
  const req = new Request("http://example.com/", { method: "POST", body: null });
  console.log(req.body === null);
  console.log(req.bodyUsed);
  console.log(await req.text());
  console.log(req.bodyUsed);
}
test();
`,
		expected: "true\nfalse\n\ntrue\ntrue\nfalse\n\ntrue\n",
	})
}

func TestLinuxAMD64WinterTCRequestInitObjectVariable(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_request_init_object_variable",
		source: `
async function test(): Promise<void> {
  const init = {
    method: "post",
    body: "payload",
    headers: { "X-Init": "yes", "Content-Type": "text/plain" }
  };
  const request = new Request("HTTP://Example.COM:80/path", init);
  console.log(request.method);
  console.log(request.url);
  console.log(request.headers.get("x-init"));
  console.log(request.headers.get("content-type"));
  console.log(await request.text());

  const source = new Request("http://example.com/source", { method: "POST", body: "old", headers: { "X-Old": "1" } });
  const override = { method: "PUT", body: "new", headers: { "X-New": "2" } };
  const copied = new Request(source, override);
  console.log(copied.method);
  console.log(copied.headers.get("x-old"));
  console.log(copied.headers.get("x-new"));
  console.log(await copied.text());

  const invalid = { method: "GET", body: "bad" };
  try {
    new Request("http://example.com/", invalid);
    console.log("invalid:unexpected");
  } catch (err: any) {
    console.log("invalid:" + err.name);
  }
}
test();
`,
		expected: "POST\nhttp://example.com/path\nyes\ntext/plain\npayload\nPUT\nnull\n2\nnew\ninvalid:TypeError\n",
	})
}
