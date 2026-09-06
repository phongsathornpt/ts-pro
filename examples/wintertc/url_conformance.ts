// WinterTC Minimum Common Web API - URL Conformance Surface
// ECMA-429 § 7.3 URL Conformance (Static Methods, Component Setters, Live Synchronization)

// 1. Static methods: URL.canParse
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

// 2. Static methods: URL.parse
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

// 3. Setters updating the URL and keeping href, toString, and searchParams in sync
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
console.log(u.toString());

u.host = "test.net:3000";
console.log(u.host);
console.log(u.hostname);
console.log(u.port);
console.log(u.href);
console.log(u.toString());

u.hostname = "sub.test.net";
console.log(u.hostname);
console.log(u.host);
console.log(u.href);
console.log(u.toString());

u.port = "9090";
console.log(u.port);
console.log(u.host);
console.log(u.href);
console.log(u.toString());

u.pathname = "/final/route";
console.log(u.pathname);
console.log(u.href);
console.log(u.toString());

u.hash = "#done";
console.log(u.hash);
console.log(u.href);
console.log(u.toString());

u.search = "?alpha=1&beta=2";
console.log(u.search);
console.log(u.href);
console.log(u.toString());
console.log(u.searchParams.get("alpha"));
console.log(sp.get("beta"));

sp.set("gamma", "3");
console.log(u.search);
console.log(u.href);
console.log(u.toString());

// 4. Setter normalization
// Port 80 on http / 443 on https becoming default ("")
const normHTTP = new URL("http://example.com/a");
normHTTP.port = "8080";
console.log(normHTTP.port);
normHTTP.port = "80";
console.log(normHTTP.port === "" ? "default" : normHTTP.port);
console.log(normHTTP.host);
console.log(normHTTP.href);

const normHTTPS = new URL("https://example.com/b");
normHTTPS.port = "8443";
console.log(normHTTPS.port);
normHTTPS.port = "443";
console.log(normHTTPS.port === "" ? "default" : normHTTPS.port);
console.log(normHTTPS.host);
console.log(normHTTPS.href);

// Pathname prepending "/" if missing and resolving relative segments
const normPath = new URL("http://example.com/start");
normPath.pathname = "no-leading-slash";
console.log(normPath.pathname);
console.log(normPath.href);

normPath.pathname = "/x/./y/../z";
console.log(normPath.pathname);
console.log(normPath.href);

normPath.pathname = "sub/./dir/../file.txt";
console.log(normPath.pathname);
console.log(normPath.href);

// Hash stripping "#"
const normHash = new URL("http://example.com/a");
normHash.hash = "#heading";
console.log(normHash.hash);
console.log(normHash.href);

normHash.hash = "subheading";
console.log(normHash.hash);
console.log(normHash.href);

normHash.hash = "";
console.log(normHash.hash === "" ? "empty" : normHash.hash);
console.log(normHash.href);

// Protocol stripping ":" and lowercasing
const normProto = new URL("http://example.com/a");
normProto.protocol = "HTTPS:";
console.log(normProto.protocol);
console.log(normProto.href);

normProto.protocol = "http";
console.log(normProto.protocol);
console.log(normProto.href);

// Invalid port ignored
const normInvPort = new URL("http://example.com:9000/a");
normInvPort.port = "invalid";
console.log(normInvPort.port);
normInvPort.port = "65536";
console.log(normInvPort.port);
normInvPort.port = "-1";
console.log(normInvPort.port);
console.log(normInvPort.href);

// Invalid protocol ignored
const normInvProto = new URL("http://example.com/a");
normInvProto.protocol = "invalid";
console.log(normInvProto.protocol);
normInvProto.protocol = "123";
console.log(normInvProto.protocol);
console.log(normInvProto.href);

// Invalid hostname ignored
const normInvHost = new URL("http://valid.test/a");
normInvHost.hostname = "bad:host";
console.log(normInvHost.hostname);
normInvHost.hostname = "";
console.log(normInvHost.hostname);
console.log(normInvHost.href);

// Invalid href throwing TypeError
try {
  u.href = "://bad";
  console.log("bad");
} catch (err) {
  console.log(err.name);
}
console.log(u.href);
