package e2e_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/phongsathornpt/ts-pro/pkg/tspro"
)

func TestLinuxAMD64WinterTCPerformanceNow(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_performance_now",
		source: `
const origin = performance.timeOrigin;
const start = performance.now();
const end = performance.now();
const json = performance.toJSON();
console.log(origin > 1600000000000);
console.log(start >= 0);
console.log(start < 60000);
console.log(end >= start);
console.log(json.timeOrigin === origin);
`,
		expected: "true\ntrue\ntrue\ntrue\ntrue\n",
	})
}

func TestLinuxAMD64WinterTCDOMException(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_dom_exception",
		source: `
const defaultError = new DOMException();
console.log(defaultError.name);
console.log(defaultError.message);
try {
  throw new DOMException("bad byte", "InvalidCharacterError");
} catch (err: any) {
  console.log(err.name);
  console.log(err.message);
}
`,
		expected: "Error\n\nInvalidCharacterError\nbad byte\n",
	})
}

func TestLinuxAMD64WinterTCNavigatorUserAgent(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_navigator_user_agent",
		source:   `console.log(navigator.userAgent);`,
		expected: "ts-pro\n",
	})
}

func TestLinuxAMD64WinterTCGlobalScopeIdentity(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_global_scope_identity",
		source: `
console.log(globalThis === self);
const g: any = globalThis;
g.marker = "alive";
for (let i = 0; i < 50000; i = i + 1) {
  const dead = "value-" + i;
}
const s: any = self;
console.log(s.marker);
`,
		expected: "true\nalive\n",
	})
}

func TestLinuxAMD64WinterTCBase64(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_base64",
		source: `
console.log(btoa("hello"));
console.log(btoa("ÿ"));
console.log(atob("aGVsbG8="));
console.log(atob("YQ"));
console.log(atob(" YW Jj\n"));
console.log(atob("/w=="));
try { console.log(btoa("✓")); } catch (err: any) { console.log(err.name); }
try { console.log(atob("A")); } catch (err: any) { console.log(err.name); }
`,
		expected: "aGVsbG8=\n/w==\nhello\na\nabc\nÿ\nInvalidCharacterError\nInvalidCharacterError\n",
	})
}

func TestLinuxAMD64TopLevelThrowExitsCleanly(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("requires linux/amd64 execution")
	}
	dir := t.TempDir()
	binPath := filepath.Join(dir, "top-level-throw")
	compiler := tspro.New(tspro.Options{TargetOS: "linux", TargetArch: "amd64", OptLevel: 2})
	bin, diags, err := compiler.CompileSource("top-level-throw.ts", []byte(`throw new DOMException("bad", "InvalidStateError");`))
	if err != nil {
		t.Fatalf("compile top-level throw: %v, diagnostics: %s", err, diags.Format(compiler.FileSet()))
	}
	if err := os.WriteFile(binPath, bin, 0o755); err != nil {
		t.Fatalf("write top-level throw binary: %v", err)
	}
	cmd := exec.Command(binPath)
	err = cmd.Run()
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("top-level throw exit = %v, want exit code 1", err)
	}
}

func TestLinuxAMD64WinterTCQueueMicrotaskOrdering(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_queue_microtask",
		source: `
console.log("sync");
queueMicrotask((): void => { console.log("micro-1"); });
queueMicrotask((): void => { console.log("micro-2"); });
spawn((): void => { console.log("task"); });
`,
		expected: "sync\nmicro-1\nmicro-2\ntask\n",
	})
}

func TestLinuxAMD64WinterTCTimerOrderingAndCancellation(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_timers",
		source: `
console.log("sync");
const cancelled = setTimeout((): void => { console.log("cancelled"); }, 0);
clearTimeout(cancelled);
setTimeout((): void => { console.log("timer"); }, 0);
queueMicrotask((): void => { console.log("micro"); });
`,
		expected: "sync\nmicro\ntimer\n",
	})
}

func TestLinuxAMD64WinterTCEventTarget(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_event_target",
		source: `
const target = new EventTarget();
const first = (event: Event): void => { console.log("first"); };
const once = (event: Event): void => { console.log(event.type); event.preventDefault(); };
target.addEventListener("tick", first);
target.addEventListener("tick", first);
target.addEventListener("tick", once, { once: true });
console.log(target.dispatchEvent(new Event("tick", { cancelable: true })));
console.log(target.dispatchEvent(new Event("tick", { cancelable: true })));
target.removeEventListener("tick", first);
console.log(target.dispatchEvent(new Event("tick", { cancelable: true })));
const stopped = new EventTarget();
const stopper = (event: Event): void => { console.log("stop"); event.stopImmediatePropagation(); };
const skipped = (event: Event): void => console.log("skipped");
stopped.addEventListener("go", stopper);
stopped.addEventListener("go", skipped);
console.log(stopped.dispatchEvent(new Event("go")));
`,
		expected: "first\ntick\nfalse\nfirst\ntrue\ntrue\nstop\ntrue\n",
	})
}

func TestLinuxAMD64WinterTCEventVariants(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_event_variants",
		source: `
const custom = new CustomEvent("custom", { detail: "payload" });
console.log(custom.type);
console.log(custom.detail);
const message = new MessageEvent("message", { data: "hello", origin: "https://example.test", lastEventId: "42" });
console.log(message.data);
console.log(message.origin);
console.log(message.lastEventId);
const error = new ErrorEvent("error", { message: "boom", filename: "app.ts", lineno: 12, colno: 7, error: "reason" });
console.log(error.message);
console.log(error.filename);
console.log(error.lineno);
console.log(error.colno);
console.log(error.error);
`,
		expected: "custom\npayload\nhello\nhttps://example.test\n42\nboom\napp.ts\n12\n7\nreason\n",
	})
}

func TestLinuxAMD64WinterTCAbortController(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_abort_controller",
		source:   mustReadExample(t, "../../examples/wintertc/abort.ts"),
		expected: "false\nabort\ntrue\nstop\nstop\nstop\nAbortError\nonabort\n",
	})
}

func TestLinuxAMD64WinterTCAbortSignalStatics(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_abort_signal_statics",
		source:   mustReadExample(t, "../../examples/wintertc/abort_static.ts"),
		expected: "true\nstatic\nAbortError\nsecond\nsecond\nfirst\nTimeoutError\n",
	})
}

func TestLinuxAMD64WinterTCMutableCaptureCells(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_mutable_capture_cells",
		source: `
let value = 1;
const arrow = (): void => { value = value + 1; };
arrow();
console.log(value);
const fn = function (): void { value = value + 3; };
fn();
console.log(value);
const nested = (): void => {
  const inner = (): void => { value++; };
  inner();
};
nested();
console.log(value);
for (let i = 0; i < 50000; i = i + 1) {
  const dead = "capture-pressure-" + i;
}
setTimeout((): void => { value = value + 4; console.log(value); }, 0);
`,
		expected: "2\n5\n6\n10\n",
	})
}

func TestLinuxAMD64WinterTCEventListenerOptions(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_event_listener_options",
		source: `
const target = new EventTarget();
const callback = (event: Event): void => { console.log("callback"); };
target.addEventListener("tick", callback, { capture: false });
target.addEventListener("tick", callback, { capture: false });
target.addEventListener("tick", callback, { capture: true });
target.dispatchEvent(new Event("tick"));
target.removeEventListener("tick", callback, { capture: false });
target.dispatchEvent(new Event("tick"));
target.removeEventListener("tick", callback, { capture: true });
const passive = new Event("passive", { cancelable: true });
target.addEventListener("passive", (event: Event): void => { event.preventDefault(); }, { passive: true });
console.log(target.dispatchEvent(passive));
console.log(passive.defaultPrevented);
const active = new Event("active", { cancelable: true });
target.addEventListener("active", (event: Event): void => { event.preventDefault(); });
console.log(target.dispatchEvent(active));
console.log(active.defaultPrevented);
`,
		expected: "callback\ncallback\ncallback\ntrue\nfalse\nfalse\ntrue\n",
	})
}

func TestLinuxAMD64WinterTCEventSignalOption(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_event_signal_option",
		source: `
const target = new EventTarget();
const controller = new AbortController();
let calls = 0;
const callback = (event: Event): void => {
  calls = calls + 1;
  console.log("signal-listener");
};
target.addEventListener("tick", callback, { signal: controller.signal });
target.dispatchEvent(new Event("tick"));
controller.abort();
target.dispatchEvent(new Event("tick"));
console.log(calls);
const already = new AbortController();
already.abort();
target.addEventListener("late", (event: Event): void => { console.log("should-not-run"); }, { signal: already.signal });
target.dispatchEvent(new Event("late"));
console.log("done");
`,
		expected: "signal-listener\n1\ndone\n",
	})
}

func TestLinuxAMD64WinterTCWebIDLBooleanCoercion(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_webidl_boolean_coercion",
		source: `
console.log(new Event("one", { cancelable: 1 }).cancelable);
console.log(new Event("zero", { cancelable: 0 }).cancelable);
console.log(new Event("text", { cancelable: "yes" }).cancelable);
console.log(new Event("empty", { cancelable: "" }).cancelable);
console.log(new Event("object", { cancelable: { value: 1 } }).cancelable);
const target = new EventTarget();
const passive = new Event("passive", { cancelable: 1 });
target.addEventListener("passive", (event: Event): void => { event.preventDefault(); }, { passive: "yes" });
console.log(target.dispatchEvent(passive));
console.log(passive.defaultPrevented);
`,
		expected: "true\nfalse\ntrue\nfalse\ntrue\ntrue\nfalse\n",
	})
}

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

func TestLinuxAMD64WinterTCURLSearchParamsCore(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_url_search_params_core",
		source: `
const p = new URLSearchParams();
p.append("a", "1");
p.append("b", "2");
p.append("a", "3");
console.log(p.size);
console.log(p.get("a"));
console.log(p.get("missing"));
console.log(p.has("a"));
const before = p.getAll("a");
console.log(before.length);
console.log(before[0]);
console.log(before[1]);
p.set("a", "9");
console.log(p.size);
console.log(p.get("a"));
const after = p.getAll("a");
console.log(after.length);
console.log(after[0]);
p.delete("b");
console.log(p.size);
console.log(p.has("b"));
p.set("c", "4");
console.log(p.size);
console.log(p.get("c"));
`,
		expected: "3\n1\nnull\ntrue\n2\n1\n3\n2\n9\n1\n9\n1\nfalse\n2\n4\n",
	})
}

func TestLinuxAMD64WinterTCURLSearchParamsFormEncoding(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_url_search_params_form_encoding",
		source: `
const params = new URLSearchParams("a=1&b=hello+world&a=%E2%9C%93&plus=%2B&empty");
console.log(params.size);
console.log(params.get("b"));
const all = params.getAll("a");
console.log(all.length);
console.log(all[1]);
console.log(params.get("empty"));
console.log(params.toString());
params.set("a", "x y");
params.delete("b");
params.append("c", "a+b");
console.log(params.toString());
`,
		expected: "5\nhello world\n2\n✓\n\na=1&b=hello+world&a=%E2%9C%93&plus=%2B&empty=\na=x+y&plus=%2B&empty=&c=a%2Bb\n",
	})
}

func TestLinuxAMD64WinterTCURLSearchParamsSort(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_url_search_params_sort",
		source: `
const params = new URLSearchParams();
params.append("", "late");
params.append("😀", "first");
params.append("a", "ascii");
params.append("😀", "second");
params.sort();
console.log(params.toString());
console.log("😀" < "");
console.log("" > "😀");
`,
		expected: "a=ascii&%F0%9F%98%80=first&%F0%9F%98%80=second&%EE%80%80=late\ntrue\ntrue\n",
	})
}

func TestLinuxAMD64WinterTCURLSearchParamsSync(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_url_search_params_sync",
		source: `
const url = new URL("https://example.com/a/b?old=1#frag");
const params = url.searchParams;
console.log(params.get("old"));
console.log(url.search);
params.append("new", "a b");
console.log(url.search);
console.log(url.href);
params.delete("old");
console.log(url.search);
params.set("new", "z");
params.sort();
console.log(url.search);
url.search = "?q=9&r=0";
console.log(url.searchParams.toString());
console.log(url.searchParams.get("q"));
console.log(url.href);
url.search = "k=1";
console.log(url.search);
console.log(url.searchParams.size);
const bare = new URL("https://example.com/x");
console.log(bare.searchParams.size);
bare.searchParams.append("a", "1");
console.log(bare.search);
console.log(bare.href);
`,
		expected: "1\n?old=1\n?old=1&new=a+b\nhttps://example.com/a/b?old=1&new=a+b#frag\n?new=a+b\n?new=z\nq=9&r=0\n9\nhttps://example.com/a/b?q=9&r=0#frag\n?k=1\n1\n0\n?a=1\nhttps://example.com/x?a=1\n",
	})
}

func TestLinuxAMD64WinterTCURLAbsoluteCore(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_url_absolute_core",
		source: `
const u = new URL("https://EXAMPLE.COM:8443/a/b?x=1#top");
console.log(u.href);
console.log(u.origin);
console.log(u.protocol);
console.log(u.host);
console.log(u.hostname);
console.log(u.port);
console.log(u.pathname);
console.log(u.search);
console.log(u.hash);
console.log(u.toString());
const a = new URL("HTTP://EXAMPLE.COM:80/a");
console.log(a.href);
console.log(a.port);
const b = new URL("https://EXAMPLE.COM:443");
console.log(b.href);
console.log(b.port);
try { new URL("ftp://example.com/file"); console.log("bad"); }
catch (err) { console.log(err.name); }
`,
		expected: "https://example.com:8443/a/b?x=1#top\nhttps://example.com:8443\nhttps:\nexample.com:8443\nexample.com\n8443\n/a/b\n?x=1\n#top\nhttps://example.com:8443/a/b?x=1#top\nhttp://example.com/a\n\nhttps://example.com/\n\nTypeError\n",
	})
}

func TestLinuxAMD64WinterTCURLRelativeResolution(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_url_relative_resolution",
		source: `
const base = new URL("https://example.com/a/b/c?old=1#old");
console.log(new URL("child", base).href);
console.log(new URL("../up", base).href);
console.log(new URL("/root/./x/../y", base).href);
console.log(new URL("?q=2", base).href);
console.log(new URL("#frag", base).href);
console.log(new URL("//other.example/x", base).href);
console.log(new URL("", base).href);
console.log(new URL("leaf", "https://example.com/a/b/").href);
`,
		expected: "https://example.com/a/b/child\nhttps://example.com/a/up\nhttps://example.com/root/y\nhttps://example.com/a/b/c?q=2\nhttps://example.com/a/b/c?old=1#frag\nhttps://other.example/x\nhttps://example.com/a/b/c?old=1\nhttps://example.com/a/b/leaf\n",
	})
}

func TestLinuxAMD64WinterTCURLStaticMethods(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_url_static_methods",
		source: `
console.log(URL.canParse("https://example.com/a/b?c=1#frag"));
console.log(URL.canParse("http://localhost:8080"));
console.log(URL.canParse("://bad"));
console.log(URL.canParse("not a url"));
console.log(URL.canParse("/relative"));
console.log(URL.canParse("child", "https://example.com/base/"));
console.log(URL.canParse("../sibling", "https://example.com/base/child/"));
const parseBase = new URL("https://example.com/base/");
console.log(URL.canParse("child", parseBase));
console.log(URL.canParse("../sibling", parseBase));
console.log(URL.canParse("child", "invalid-base"));

function testParse(url: string): void {
  const p = URL.parse(url);
  if (p === null) {
    console.log("null");
    return;
  }
  console.log(p.href);
}

function testParseWithBase(url: string, base: string): void {
  const p = URL.parse(url, base);
  if (p === null) {
    console.log("null");
    return;
  }
  console.log(p.href);
}

function testParseWithURLBase(url: string, base: URL): void {
  const p = URL.parse(url, base);
  if (p === null) {
    console.log("null");
    return;
  }
  console.log(p.href);
}

testParse("https://example.com/foo?bar=1#baz");
testParseWithBase("sub/page", "https://example.com/root/");
testParseWithURLBase("other", parseBase);
testParse("not a url");
testParse("/relative");
testParseWithBase("child", "invalid-base");
`,
		expected: "true\ntrue\nfalse\nfalse\nfalse\ntrue\ntrue\ntrue\ntrue\nfalse\nhttps://example.com/foo?bar=1#baz\nhttps://example.com/root/sub/page\nhttps://example.com/base/other\nnull\nnull\nnull\n",
	})
}

func TestLinuxAMD64WinterTCURLSetters(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_url_setters",
		source: `
const u = new URL("https://example.com:8443/old/path?old=1#oldfrag");
const sp = u.searchParams;

u.href = "http://example.org:8080/new/path?new=2#newfrag";
console.log(u.href);
console.log(u.toString());
console.log(u.protocol);
console.log(u.host);
console.log(u.hostname);
console.log(u.port);
console.log(u.pathname);
console.log(u.search);
console.log(u.hash);
console.log(u.searchParams.get("new"));
console.log(sp.get("new"));

u.protocol = "https:";
console.log(u.protocol);
console.log(u.href);

u.host = "test.net:3000";
console.log(u.host);
console.log(u.hostname);
console.log(u.port);
console.log(u.href);

u.hostname = "sub.test.net";
console.log(u.hostname);
console.log(u.host);
console.log(u.href);

u.port = "9090";
console.log(u.port);
console.log(u.host);
console.log(u.href);

u.pathname = "/final/route";
console.log(u.pathname);
console.log(u.href);

u.hash = "#done";
console.log(u.hash);
console.log(u.href);

u.search = "?alpha=1&beta=2";
console.log(u.search);
console.log(u.href);
console.log(u.searchParams.get("alpha"));
console.log(sp.get("beta"));

sp.set("gamma", "3");
console.log(u.search);
console.log(u.href);

// normalization tests
const normHTTP = new URL("http://example.com/a");
normHTTP.port = "80";
console.log(normHTTP.port === "" ? "default" : normHTTP.port);
console.log(normHTTP.host);

const normHTTPS = new URL("https://example.com/b");
normHTTPS.port = "443";
console.log(normHTTPS.port === "" ? "default" : normHTTPS.port);
console.log(normHTTPS.host);

const normPath = new URL("http://example.com/start");
normPath.pathname = "sub/./dir/../file.txt";
console.log(normPath.pathname);

const normHash = new URL("http://example.com/a");
normHash.hash = "heading";
console.log(normHash.hash);
normHash.hash = "";
console.log(normHash.hash === "" ? "empty" : normHash.hash);

const normProto = new URL("http://example.com/a");
normProto.protocol = "HTTPS:";
console.log(normProto.protocol);
normProto.protocol = "invalid";
console.log(normProto.protocol);

const normPort = new URL("http://example.com:9000/a");
normPort.port = "invalid";
console.log(normPort.port);

const normHost = new URL("http://valid.test/a");
normHost.hostname = "bad:host";
console.log(normHost.hostname);

try {
  u.href = "://bad";
  console.log("bad");
} catch (err) {
  console.log(err.name);
}
console.log(u.href);
`,
		expected: "http://example.org:8080/new/path?new=2#newfrag\nhttp://example.org:8080/new/path?new=2#newfrag\nhttp:\nexample.org:8080\nexample.org\n8080\n/new/path\n?new=2\n#newfrag\n2\n2\nhttps:\nhttps://example.org:8080/new/path?new=2#newfrag\ntest.net:3000\ntest.net\n3000\nhttps://test.net:3000/new/path?new=2#newfrag\nsub.test.net\nsub.test.net:3000\nhttps://sub.test.net:3000/new/path?new=2#newfrag\n9090\nsub.test.net:9090\nhttps://sub.test.net:9090/new/path?new=2#newfrag\n/final/route\nhttps://sub.test.net:9090/final/route?new=2#newfrag\n#done\nhttps://sub.test.net:9090/final/route?new=2#done\n?alpha=1&beta=2\nhttps://sub.test.net:9090/final/route?alpha=1&beta=2#done\n1\n2\n?alpha=1&beta=2&gamma=3\nhttps://sub.test.net:9090/final/route?alpha=1&beta=2&gamma=3#done\ndefault\nexample.com\ndefault\nexample.com\n/sub/file.txt\n#heading\nempty\nhttps:\nhttps:\n9000\nvalid.test\nTypeError\nhttps://sub.test.net:9090/final/route?alpha=1&beta=2&gamma=3#done\n",
	})
}

func TestLinuxAMD64WinterTCURLConformance(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_url_conformance",
		source:   mustReadExample(t, "../../examples/wintertc/url_conformance.ts"),
		expected: "true\ntrue\nfalse\nfalse\nfalse\ntrue\ntrue\ntrue\ntrue\nfalse\nhttps://example.com/foo?bar=1#baz\nhttps://example.com/root/sub/page\nhttps://example.com/base/other\nnull\nnull\nnull\nhttp://example.org:8080/new/path?new=2#newfrag\nhttp://example.org:8080/new/path?new=2#newfrag\nhttp:\nexample.org:8080\nexample.org\n8080\n/new/path\n?new=2\n#newfrag\n2\n2\nhttps:\nhttps://example.org:8080/new/path?new=2#newfrag\nhttps://example.org:8080/new/path?new=2#newfrag\ntest.net:3000\ntest.net\n3000\nhttps://test.net:3000/new/path?new=2#newfrag\nhttps://test.net:3000/new/path?new=2#newfrag\nsub.test.net\nsub.test.net:3000\nhttps://sub.test.net:3000/new/path?new=2#newfrag\nhttps://sub.test.net:3000/new/path?new=2#newfrag\n9090\nsub.test.net:9090\nhttps://sub.test.net:9090/new/path?new=2#newfrag\nhttps://sub.test.net:9090/new/path?new=2#newfrag\n/final/route\nhttps://sub.test.net:9090/final/route?new=2#newfrag\nhttps://sub.test.net:9090/final/route?new=2#newfrag\n#done\nhttps://sub.test.net:9090/final/route?new=2#done\nhttps://sub.test.net:9090/final/route?new=2#done\n?alpha=1&beta=2\nhttps://sub.test.net:9090/final/route?alpha=1&beta=2#done\nhttps://sub.test.net:9090/final/route?alpha=1&beta=2#done\n1\n2\n?alpha=1&beta=2&gamma=3\nhttps://sub.test.net:9090/final/route?alpha=1&beta=2&gamma=3#done\nhttps://sub.test.net:9090/final/route?alpha=1&beta=2&gamma=3#done\n8080\ndefault\nexample.com\nhttp://example.com/a\n8443\ndefault\nexample.com\nhttps://example.com/b\n/no-leading-slash\nhttp://example.com/no-leading-slash\n/x/z\nhttp://example.com/x/z\n/sub/file.txt\nhttp://example.com/sub/file.txt\n#heading\nhttp://example.com/a#heading\n#subheading\nhttp://example.com/a#subheading\nempty\nhttp://example.com/a\nhttps:\nhttps://example.com/a\nhttp:\nhttp://example.com/a\n9000\n9000\n9000\nhttp://example.com:9000/a\nhttp:\nhttp:\nhttp://example.com/a\nvalid.test\nvalid.test\nhttp://valid.test/a\nTypeError\nhttps://sub.test.net:9090/final/route?alpha=1&beta=2&gamma=3#done\n",
	})
}

func TestLinuxAMD64WinterTCURLPattern(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_url_pattern_basic",
		source: `
const p = new URLPattern("https://example.com/books/:id");
const r = p.exec("https://example.com/books/42");
console.log(r.pathname.input);
const g: any = r.pathname.groups;
console.log(g.id);
`,
		expected: "/books/42\n42\n",
	})
}

func TestLinuxAMD64WinterTCURLPatternFullConformance(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_url_pattern_conformance",
		source:   mustReadExample(t, "../../examples/wintertc/url_pattern_conformance.ts"),
		expected: "https\nexample.com\n8080\n/books/:id\nsort=:sort\nsection\nfalse\nhttps\nexample.org\n/api/:version/*\n*\n*\n/users/:id\nhttps\nexample.net\n3000\n/articles/:slug\nTypeError\nTypeError\ntrue\nfalse\nfalse\ntrue\nfalse\ntrue\ntrue\nfalse\ntrue\nfalse\n/books/42\n42\nasc\n1\nhttps://example.com:8080/books/42?sort=asc#section\nv1\nposts/100\ntrue\ntrue\ntrue\nfalse\n777\ntrue\ntrue\ntrue\nfalse\napi\n",
	})
}

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

func TestLinuxAMD64WinterTCQueuingStrategies(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_queuing_strategies",
		source: `
const bl = new ByteLengthQueuingStrategy({ highWaterMark: 512 });
console.log(bl.highWaterMark);
const u8 = new Uint8Array(8);
console.log(bl.size(u8));

const cs = new CountQueuingStrategy({ highWaterMark: 3 });
console.log(cs.highWaterMark);
console.log(cs.size("test"));
`,
		expected: "512\n8\n3\n1\n",
	})
}

func TestLinuxAMD64WinterTCReadableStreamCore(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_readable_stream_core",
		source: `
async function test(): Promise<void> {
  const rs = new ReadableStream({
    start: (controller: ReadableStreamDefaultController): void => {
      console.log(controller.desiredSize);
      controller.enqueue("a");
      controller.enqueue("b");
      controller.close();
    }
  });
  console.log(rs.locked);
  const reader = rs.getReader();
  console.log(rs.locked);

  const r1 = await reader.read();
  console.log(r1.value);
  console.log(r1.done);

  const r2 = await reader.read();
  console.log(r2.value);
  console.log(r2.done);

  const r3 = await reader.read();
  console.log(r3.done);

  reader.releaseLock();
  console.log(rs.locked);
}
test();
`,
		expected: "1\nfalse\ntrue\na\nfalse\nb\nfalse\ntrue\nfalse\n",
	})
}

func TestLinuxAMD64WinterTCWritableStreamCore(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_writable_stream_core",
		source: `
async function test(): Promise<void> {
  const items: string[] = [];
  const ws = new WritableStream({
    write: (chunk: any): void => {
      items.push(chunk);
    }
  });
  console.log(ws.locked);
  const writer = ws.getWriter();
  console.log(ws.locked);
  await writer.write("msg1");
  await writer.write("msg2");
  await writer.close();
  console.log(items.length);
  console.log(items[0]);
  console.log(items[1]);
}
test();
`,
		expected: "false\ntrue\n2\nmsg1\nmsg2\n",
	})
}

func TestLinuxAMD64WinterTCTransformStreamPipe(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_transform_stream_pipe",
		source: `
async function test(): Promise<void> {
  const ts = new TransformStream({
    transform: (chunk: any, controller: any): void => {
      controller.enqueue("got:" + chunk);
    }
  });
  const rs = ReadableStream.from(["one", "two"]);
  const piped = rs.pipeThrough(ts);
  const reader = piped.getReader();
  const c1 = await reader.read();
  console.log(c1.value);
  const c2 = await reader.read();
  console.log(c2.value);
  const c3 = await reader.read();
  console.log(c3.done);
}
test();
`,
		expected: "got:one\ngot:two\ntrue\n",
	})
}

func TestLinuxAMD64WinterTCStreamsConformance(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_streams_conformance",
		source:   mustReadExample(t, "../../examples/wintertc/streams_conformance.ts"),
		expected: "1024\n16\n10\n1\n1\nfalse\ntrue\nchunk1\nfalse\nchunk2\nfalse\ntrue\nfalse\nalpha\nbeta\ntrue\ntrue\nbranch-data\nbranch-data\nfalse\ntrue\n1\n2\nwrite-1\nwrite-2\ntransformed:in1\ntransformed:in2\ntrue\nutf-8\nutf-8\nfalse\nfalse\nblob-stream-content\ntrue\n",
	})
}

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

func TestLinuxAMD64WinterTCFetchLoopbackTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("X-Test", " one ")
		w.Header().Add("X-Test", "two")
		w.Header().Add("Set-Cookie", "a=1")
		w.Header().Add("Set-Cookie", "b=2")
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(w, "%s|%s", r.Method, r.UserAgent())
	}))
	defer server.Close()

	source := fmt.Sprintf(`
async function test(): Promise<void> {
  const res = await fetch(%q);
  console.log(res.status);
  console.log(res.ok);
  console.log(res.url);
  console.log(res.headers.get("x-test"));
  const cookies = res.headers.getSetCookie();
  console.log(cookies.length);
  console.log(cookies[0]);
  console.log(cookies[1]);
  console.log(await res.text());
}
test();
`, server.URL+"/hello?x=1")
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_fetch_loopback_transport",
		source:   source,
		expected: "201\ntrue\n" + server.URL + "/hello?x=1\none, two\n2\na=1\nb=2\nGET|ts-pro\n",
	})
}

func TestLinuxAMD64WinterTCFetchRequestNormalization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		w.WriteHeader(http.StatusAccepted)
		_, _ = fmt.Fprintf(w, "%s|%s|%s|%s", r.Method, r.Header.Get("X-Test"), string(body), r.UserAgent())
	}))
	defer server.Close()

	source := fmt.Sprintf(`
async function test(): Promise<void> {
  const req = new Request(%q, {
    method: "post",
    headers: { "X-Test": "alpha" },
    body: "payload"
  });
  const res = await fetch(req, {
    method: "put",
    headers: { "X-Test": "beta" },
    body: "override"
  });
  console.log(req.bodyUsed);
  console.log(res.status);
  console.log(await res.text());
}
test();
`, server.URL+"/submit")
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_fetch_request_normalization",
		source:   source,
		expected: "true\n202\nPUT|beta|override|ts-pro\n",
	})
}
