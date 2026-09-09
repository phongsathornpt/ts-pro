package wintertc_test

import (
	"testing"
)

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
