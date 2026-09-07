package e2e_test

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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

func TestLinuxAMD64WinterTCFetchPreAbortedSignal(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	source := fmt.Sprintf(`
async function test(): Promise<void> {
  const controller = new AbortController();
  controller.abort();
  try {
    await fetch(%q, { signal: controller.signal });
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }
}
test();
`, server.URL+"/abort")
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_fetch_pre_aborted_signal",
		source:   source,
		expected: "AbortError\n",
	})
	if got := hits.Load(); got != 0 {
		t.Fatalf("pre-aborted fetch reached server %d times", got)
	}
}

func TestLinuxAMD64WinterTCFetchRedirectFollow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			w.Header().Set("Location", "/final")
			w.WriteHeader(http.StatusFound)
			return
		}
		w.Header().Set("X-Final", "yes")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "redirected-body")
	}))
	defer server.Close()

	source := fmt.Sprintf(`
async function test(): Promise<void> {
  const res = await fetch(%q);
  console.log(res.status);
  console.log(res.redirected);
  console.log(res.url);
  console.log(res.headers.get("x-final"));
  console.log(await res.text());
}
test();
`, server.URL+"/start")
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_fetch_redirect_follow",
		source:   source,
		expected: "200\ntrue\n" + server.URL + "/final\nyes\nredirected-body\n",
	})
}

func TestLinuxAMD64WinterTCFetchRedirectMethodSemantics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start303":
			w.Header().Set("Location", "/final303")
			w.WriteHeader(http.StatusSeeOther)
		case "/start307":
			w.Header().Set("Location", "/final307")
			w.WriteHeader(http.StatusTemporaryRedirect)
		default:
			body := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(body)
			_, _ = fmt.Fprintf(w, "%s|%s", r.Method, string(body))
		}
	}))
	defer server.Close()

	source := fmt.Sprintf(`
async function test(): Promise<void> {
  const a = await fetch(%q, { method: "POST", body: "payload" });
  console.log(await a.text());
  const b = await fetch(%q, { method: "POST", body: "payload" });
  console.log(await b.text());
}
test();
`, server.URL+"/start303", server.URL+"/start307")
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_fetch_redirect_method_semantics",
		source:   source,
		expected: "GET|\nPOST|payload\n",
	})
}

func TestLinuxAMD64WinterTCFetchRedirectModes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			w.Header().Set("Location", "/final")
			w.WriteHeader(http.StatusFound)
			return
		}
		_, _ = fmt.Fprint(w, "final")
	}))
	defer server.Close()

	source := fmt.Sprintf(`
async function test(): Promise<void> {
  const manual = await fetch(%q, { redirect: "manual" });
  console.log(manual.status);
  console.log(manual.redirected);
  console.log(manual.headers.get("location"));
  try {
    await fetch(%q, { redirect: "error" });
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }
}
test();
`, server.URL+"/start", server.URL+"/start")
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_fetch_redirect_modes",
		source:   source,
		expected: "302\nfalse\n/final\nTypeError\n",
	})
}

func TestLinuxAMD64WinterTCFetchMultiHopRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/hop/0":
			w.Header().Set("Location", "/hop/1")
			w.WriteHeader(http.StatusFound)
		case "/hop/1":
			w.Header().Set("Location", "/hop/2")
			w.WriteHeader(http.StatusTemporaryRedirect)
		case "/hop/2":
			w.Header().Set("Location", "/final")
			w.WriteHeader(http.StatusPermanentRedirect)
		default:
			w.Header().Set("X-Hop", "final")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, "multi-hop-body")
		}
	}))
	defer server.Close()

	source := fmt.Sprintf(`
async function test(): Promise<void> {
  const res = await fetch(%q);
  console.log(res.status);
  console.log(res.redirected);
  console.log(res.url);
  console.log(res.headers.get("x-hop"));
  console.log(await res.text());
}
test();
`, server.URL+"/hop/0")
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_fetch_multi_hop_redirect",
		source:   source,
		expected: "200\ntrue\n" + server.URL + "/final\nfinal\nmulti-hop-body\n",
	})
}

func TestLinuxAMD64WinterTCFetchRedirectLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/loop")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()

	source := fmt.Sprintf(`
async function test(): Promise<void> {
  try {
    await fetch(%q);
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }
}
test();
`, server.URL+"/loop")
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_fetch_redirect_limit",
		source:   source,
		expected: "TypeError\n",
	})
}

func TestLinuxAMD64WinterTCFetchInFlightAbort(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "too-late")
	}))
	defer server.Close()

	source := fmt.Sprintf(`
async function test(): Promise<void> {
  try {
    await fetch(%q, { signal: AbortSignal.timeout(5) });
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }
}
test();
`, server.URL+"/slow")
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_fetch_in_flight_abort",
		source:   source,
		expected: "TimeoutError\n",
	})
	if got := hits.Load(); got != 1 {
		t.Fatalf("in-flight abort server hits = %d, want 1", got)
	}
}

func TestLinuxAMD64WinterTCFetchLargeResponse(t *testing.T) {
	const size = 200000
	payload := strings.Repeat("x", size)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(payload)))
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, payload)
	}))
	defer server.Close()

	source := fmt.Sprintf(`
async function test(): Promise<void> {
  const res = await fetch(%q);
  const bytes = await res.bytes();
  console.log(res.status);
  console.log(bytes.length);
  console.log(bytes[0]);
  console.log(bytes[199999]);
}
test();
`, server.URL+"/large")
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_fetch_large_response",
		source:   source,
		expected: "200\n200000\n120\n120\n",
	})
}

func TestLinuxAMD64WinterTCBodyStreamBacking(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_body_stream_backing",
		source: `
function test(): void {
  const emptyResponse = new Response();
  console.log(emptyResponse.body === null);

  const response = new Response("response-body");
  console.log(response.body === null);
  console.log(response.body === response.body);

  const emptyRequest = new Request("https://example.com/");
  console.log(emptyRequest.body === null);

  const request = new Request("https://example.com/", { method: "POST", body: "request-body" });
  console.log(request.body === null);
  console.log(request.body === request.body);
}
test();
`,
		expected: "true\nfalse\ntrue\ntrue\nfalse\ntrue\n",
	})
}

func TestLinuxAMD64WinterTCBodyStreamDisturbance(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_body_stream_disturbance",
		source: `
async function test(): Promise<void> {
  const response = new Response("abc");
  console.log(response.bodyUsed);
  const responseStream = response.body ?? new ReadableStream();
  const responseReader = responseStream.getReader();
  const responseChunk: any = await responseReader.read();
  console.log(response.bodyUsed);
  const responseBytes: Uint8Array = responseChunk.value;
  console.log(responseBytes.length);
  console.log(responseBytes[0]);

  const request = new Request("https://example.com/", { method: "POST", body: "xyz" });
  console.log(request.bodyUsed);
  const requestStream = request.body ?? new ReadableStream();
  const requestReader = requestStream.getReader();
  await requestReader.cancel();
  console.log(request.bodyUsed);
}
test();
`,
		expected: "false\ntrue\n3\n97\nfalse\ntrue\n",
	})
}

func TestLinuxAMD64WinterTCBodyLockedIsUnusable(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_body_locked_unusable",
		source: `
async function test(): Promise<void> {
  const response = new Response("abc");
  const responseStream = response.body ?? new ReadableStream();
  responseStream.getReader();
  console.log(response.bodyUsed);
  try {
    await response.text();
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }
  try {
    response.clone();
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }

  const request = new Request("https://example.com/", { method: "POST", body: "xyz" });
  const requestStream = request.body ?? new ReadableStream();
  requestStream.getReader();
  console.log(request.bodyUsed);
  try {
    request.clone();
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }
}
test();
`,
		expected: "false\nTypeError\nTypeError\nfalse\nTypeError\n",
	})
}

func TestLinuxAMD64WinterTCFetchNumericIPv4Transport(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.2:0")
	if err != nil {
		t.Fatalf("listen on alternate loopback IPv4: %v", err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Host", r.Host)
		_, _ = fmt.Fprint(w, "ipv4-ok")
	})}
	defer server.Close()
	go func() { _ = server.Serve(listener) }()

	url := "http://" + listener.Addr().String() + "/numeric"
	source := fmt.Sprintf(`
async function test(): Promise<void> {
  const response = await fetch(%q);
  console.log(response.status);
  console.log(response.headers.get("x-host"));
  console.log(await response.text());
}
test();
`, url)
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_fetch_numeric_ipv4_transport",
		source:   source,
		expected: "200\n" + listener.Addr().String() + "\nipv4-ok\n",
	})
}

func TestLinuxAMD64WinterTCFetchInvalidNumericIPv4(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_fetch_invalid_numeric_ipv4",
		source: `
async function test(): Promise<void> {
  try {
    await fetch("http://999.1.1.1:8080/");
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }
}
test();
`,
		expected: "TypeError\n",
	})
}

func TestLinuxAMD64WinterTCFetchLocalhostResolver(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Resolver", "localhost")
		_, _ = fmt.Fprint(w, "resolved")
	}))
	defer server.Close()
	_, port, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatalf("split httptest address: %v", err)
	}
	url := "http://localhost:" + port + "/resolver"
	source := fmt.Sprintf(`
async function test(): Promise<void> {
  const response = await fetch(%q);
  console.log(response.status);
  console.log(response.headers.get("x-resolver"));
  console.log(await response.text());
}
test();
`, url)
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_fetch_localhost_resolver",
		source:   source,
		expected: "200\nlocalhost\nresolved\n",
	})
}

func TestLinuxAMD64WinterTCFetchHostsFileResolver(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for hosts resolver test: %v", err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Resolver", "hosts")
		_, _ = fmt.Fprint(w, "hosts-ok")
	})}
	defer server.Close()
	go func() { _ = server.Serve(listener) }()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split listener address: %v", err)
	}
	url := "http://runtime.us-east-1.kiro.dev:" + port + "/hosts"
	source := fmt.Sprintf(`
async function test(): Promise<void> {
  const response = await fetch(%q);
  console.log(response.status);
  console.log(response.headers.get("x-resolver"));
  console.log(await response.text());
}
test();
`, url)
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_fetch_hosts_file_resolver",
		source:   source,
		expected: "200\nhosts\nhosts-ok\n",
	})
}

func TestLinuxAMD64WinterTCFetchChunkedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatalf("response writer does not implement http.Flusher")
		}
		_, _ = fmt.Fprint(w, "hello-")
		flusher.Flush()
		_, _ = fmt.Fprint(w, "chunked")
		flusher.Flush()
	}))
	defer server.Close()

	source := fmt.Sprintf(`
async function test(): Promise<void> {
  const response = await fetch(%q);
  console.log(response.status);
  console.log(response.headers.get("transfer-encoding"));
  console.log(await response.text());
}
test();
`, server.URL+"/chunked")
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_fetch_chunked_response",
		source:   source,
		expected: "200\nchunked\nhello-chunked\n",
	})
}

func TestLinuxAMD64WinterTCFetchExternalDNSResolver(t *testing.T) {
	if os.Getenv("TS_PRO_EXTERNAL_DNS_TEST") != "1" {
		t.Skip("external DNS/network integration is opt-in")
	}
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_fetch_external_dns_resolver",
		source: `
async function test(): Promise<void> {
  const res = await fetch("http://example.com/", { redirect: "manual", signal: AbortSignal.timeout(3000) });
  console.log(res.status > 0);
}
test();
`,
		expected: "true\n",
	})
}

func TestLinuxAMD64WinterTCURLIPv6Authority(t *testing.T) {
	runLinuxAMD64(t, linuxAMD64Case{
		name: "wintertc_url_ipv6_authority",
		source: `
const a = new URL("http://[::1]/x");
console.log(a.hostname);
console.log(a.host);
console.log(a.href);
const b = new URL("http://[2001:db8::1]:8080/path");
console.log(b.hostname);
console.log(b.port);
console.log(b.host);
console.log(b.href);
try {
  new URL("http://[::1/path");
  console.log("unexpected");
} catch (err: any) {
  console.log(err.name);
}
`,
		expected: "[::1]\n[::1]\nhttp://[::1]/x\n[2001:db8::1]\n8080\n[2001:db8::1]:8080\nhttp://[2001:db8::1]:8080/path\nTypeError\n",
	})
}

func TestLinuxAMD64WinterTCFetchIPv6Transport(t *testing.T) {
	ln, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback unavailable: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	hostSeen := make(chan string, 1)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hostSeen <- r.Host
		w.Header().Set("X-IPv6", "yes")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = fmt.Fprint(w, "ipv6-ok")
	})}
	go func() { _ = server.Serve(ln) }()
	defer server.Close()

	url := fmt.Sprintf("http://[::1]:%d/ipv6", port)
	source := fmt.Sprintf(`
async function test(): Promise<void> {
  const res = await fetch(%q);
  console.log(res.status);
  console.log(res.url);
  console.log(res.headers.get("x-ipv6"));
  console.log(await res.text());
}
test();
`, url)
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_fetch_ipv6_transport",
		source:   source,
		expected: "206\n" + url + "\nyes\nipv6-ok\n",
	})
	select {
	case got := <-hostSeen:
		want := fmt.Sprintf("[::1]:%d", port)
		if got != want {
			t.Fatalf("IPv6 Host header: got %q, want %q", got, want)
		}
	default:
		t.Fatal("IPv6 server did not observe request")
	}
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
