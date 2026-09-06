package e2e_test

import (
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
