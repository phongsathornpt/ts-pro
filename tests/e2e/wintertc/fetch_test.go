package wintertc_test

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

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

func TestLinuxAMD64WinterTCFetchLargeRequestBody(t *testing.T) {
	const size = 200000
	var gotLen atomic.Int64
	var first atomic.Int64
	var last atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		gotLen.Store(int64(len(body)))
		if len(body) != 0 {
			first.Store(int64(body[0]))
			last.Store(int64(body[len(body)-1]))
		}
		w.Header().Set("Content-Length", "2")
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()

	source := fmt.Sprintf(`
async function test(): Promise<void> {
  const body = new Uint8Array(%d);
  body[0] = 17;
  body[%d] = 23;
  const res = await fetch(%q, { method: "POST", body: body });
  console.log(res.status);
  console.log(await res.text());
}
test();
`, size, size-1, server.URL+"/upload")
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_fetch_large_request_body",
		source:   source,
		expected: "200\nok\n",
	})
	if got := gotLen.Load(); got != size {
		t.Fatalf("request body length = %d, want %d", got, size)
	}
	if got := first.Load(); got != 17 {
		t.Fatalf("request body first byte = %d, want 17", got)
	}
	if got := last.Load(); got != 23 {
		t.Fatalf("request body last byte = %d, want 23", got)
	}
}

func TestLinuxAMD64WinterTCFetchRequestBodyUploadAbort(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		// Intentionally do not consume the request body. Once the kernel send
		// buffer fills, the client must yield and observe AbortSignal cancellation.
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	source := fmt.Sprintf(`
async function test(): Promise<void> {
  const body = new Uint8Array(8388608);
  try {
    await fetch(%q, { method: "POST", body: body, signal: AbortSignal.timeout(50) });
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }
}
test();
`, server.URL+"/blocked-upload")
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_fetch_request_body_upload_abort",
		source:   source,
		expected: "TimeoutError\n",
	})
	if got := hits.Load(); got != 1 {
		t.Fatalf("upload abort server hits = %d, want 1", got)
	}
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

func TestLinuxAMD64WinterTCFetchInitValidationBeforeNetwork(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer server.Close()

	source := fmt.Sprintf(`
async function test(): Promise<void> {
  const badRedirect = "bogus";
  try {
    await fetch(%q, { redirect: badRedirect });
    console.log("redirect:unexpected");
  } catch (err: any) {
    console.log("redirect:" + err.name);
  }
  try {
    await fetch(%q, { method: "GET", body: "payload" });
    console.log("get:unexpected");
  } catch (err: any) {
    console.log("get:" + err.name);
  }
  try {
    await fetch(%q, { method: "HEAD", body: "payload" });
    console.log("head:unexpected");
  } catch (err: any) {
    console.log("head:" + err.name);
  }
}
test();
`, server.URL, server.URL, server.URL)
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_fetch_init_validation_before_network",
		source:   source,
		expected: "redirect:TypeError\nget:TypeError\nhead:TypeError\n",
	})
	if got := hits.Load(); got != 0 {
		t.Fatalf("invalid fetch init reached network %d times", got)
	}
}

func TestLinuxAMD64WinterTCFetchStatusText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/created":
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprint(w, "created")
		case "/redirect":
			w.Header().Set("Location", "/accepted")
			w.WriteHeader(http.StatusFound)
		case "/accepted":
			w.WriteHeader(http.StatusAccepted)
			_, _ = fmt.Fprint(w, "accepted")
		}
	}))
	defer server.Close()

	source := fmt.Sprintf(`
async function test(): Promise<void> {
  const created = await fetch(%q);
  console.log(created.status);
  console.log(created.statusText);
  const accepted = await fetch(%q);
  console.log(accepted.status);
  console.log(accepted.statusText);
  console.log(accepted.redirected);
}
test();
`, server.URL+"/created", server.URL+"/redirect")
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_fetch_status_text",
		source:   source,
		expected: "201\nCreated\n202\nAccepted\ntrue\n",
	})
}

func TestLinuxAMD64WinterTCFetchRedirectBodyHeaderSemantics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start303":
			w.Header().Set("Location", "/final303")
			w.WriteHeader(http.StatusSeeOther)
			return
		case "/start307":
			w.Header().Set("Location", "/final307")
			w.WriteHeader(http.StatusTemporaryRedirect)
			return
		}
		body, _ := io.ReadAll(r.Body)
		_, _ = fmt.Fprintf(w, "%s|%s|%s|%s|%s", r.Method, string(body), r.Header.Get("Content-Type"), r.Header.Get("Content-Language"), r.Header.Get("Content-Location"))
	}))
	defer server.Close()

	source := fmt.Sprintf(`
async function test(): Promise<void> {
  console.log(await (await fetch(%q, { method: "POST", body: "payload", headers: { "Content-Type": "text/plain", "Content-Language": "en", "Content-Location": "/source" } })).text());
  console.log(await (await fetch(%q, { method: "POST", body: "payload", headers: { "Content-Type": "text/plain", "Content-Language": "en", "Content-Location": "/source" } })).text());
}
test();
`, server.URL+"/start303", server.URL+"/start307")
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_fetch_redirect_body_header_semantics",
		source:   source,
		expected: "GET||||\nPOST|payload|text/plain|en|/source\n",
	})
}

func TestLinuxAMD64WinterTCFetchInitObjectVariable(t *testing.T) {
	var finalHits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/echo":
			body, _ := io.ReadAll(r.Body)
			_, _ = fmt.Fprintf(w, "%s|%s|%s|%s", r.Method, string(body), r.Header.Get("X-Init"), r.Header.Get("Content-Type"))
		case "/redirect":
			w.Header().Set("Location", "/final")
			w.WriteHeader(http.StatusFound)
		case "/final":
			finalHits.Add(1)
			_, _ = fmt.Fprint(w, "final")
		}
	}))
	defer server.Close()

	source := fmt.Sprintf(`
async function test(): Promise<void> {
  const init = {
    method: "POST",
    body: "payload",
    headers: { "X-Init": "yes", "Content-Type": "text/plain" }
  };
  const echo = await fetch(%q, init);
  console.log(await echo.text());

  const manual = { redirect: "manual" };
  const redirect = await fetch(%q, manual);
  console.log(redirect.status);
  console.log(redirect.redirected);
  console.log(redirect.headers.get("location"));
}
test();
`, server.URL+"/echo", server.URL+"/redirect")
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_fetch_init_object_variable",
		source:   source,
		expected: "POST|payload|yes|text/plain\n302\nfalse\n/final\n",
	})
	if got := finalHits.Load(); got != 0 {
		t.Fatalf("manual redirect unexpectedly followed %d times", got)
	}
}

func TestLinuxAMD64WinterTCFetchRedirectDoesNotWaitForBody(t *testing.T) {
	finalReached := make(chan struct{}, 1)
	var waitedForBody atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			w.Header().Set("Location", "/final")
			w.WriteHeader(http.StatusFound)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			select {
			case <-finalReached:
			case <-time.After(750 * time.Millisecond):
				waitedForBody.Store(true)
			}
			_, _ = fmt.Fprint(w, "ignored redirect body")
		case "/final":
			select {
			case finalReached <- struct{}{}:
			default:
			}
			_, _ = fmt.Fprint(w, "final")
		}
	}))
	defer server.Close()

	source := fmt.Sprintf(`
async function test(): Promise<void> {
  const response = await fetch(%q);
  console.log(response.status);
  console.log(response.redirected);
  console.log(await response.text());
}
test();
`, server.URL+"/start")
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_fetch_redirect_headers_before_body",
		source:   source,
		expected: "200\ntrue\nfinal\n",
	})
	if waitedForBody.Load() {
		t.Fatal("fetch waited for redirect response body before following Location")
	}
}

func TestLinuxAMD64WinterTCFetchResolvesAfterHeaders(t *testing.T) {
	release := make(chan struct{}, 1)
	var waitedForRelease atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/body":
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Length", "5")
			w.WriteHeader(http.StatusOK)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			select {
			case <-release:
			case <-time.After(750 * time.Millisecond):
				waitedForRelease.Store(true)
			}
			_, _ = fmt.Fprint(w, "hello")
		case "/release":
			select {
			case release <- struct{}{}:
			default:
			}
			_, _ = fmt.Fprint(w, "released")
		}
	}))
	defer server.Close()

	source := fmt.Sprintf(`
async function test(): Promise<void> {
  const response = await fetch(%q);
  console.log(response.status);
  console.log(response.headers.get("content-type"));
  await fetch(%q);
  console.log(await response.text());
}
test();
`, server.URL+"/body", server.URL+"/release")
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_fetch_resolves_after_headers",
		source:   source,
		expected: "200\ntext/plain\nhello\n",
	})
	if waitedForRelease.Load() {
		t.Fatal("fetch waited for the response body before resolving")
	}
}

func TestLinuxAMD64WinterTCFetchLiveBodyReader(t *testing.T) {
	release := make(chan struct{}, 1)
	var waitedForRelease atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/body":
			w.Header().Set("Content-Length", "3")
			w.WriteHeader(http.StatusOK)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			select {
			case <-release:
			case <-time.After(750 * time.Millisecond):
				waitedForRelease.Store(true)
			}
			_, _ = w.Write([]byte{97, 0, 122})
		case "/release":
			select {
			case release <- struct{}{}:
			default:
			}
			_, _ = fmt.Fprint(w, "released")
		}
	}))
	defer server.Close()

	source := fmt.Sprintf(`
async function test(): Promise<void> {
  const response = await fetch(%q);
  console.log(response.bodyUsed);
  const stream = response.body ?? new ReadableStream();
  const reader = stream.getReader();
  console.log(response.bodyUsed);
  await fetch(%q);
  const first: any = await reader.read();
  const bytes: Uint8Array = first.value;
  console.log(first.done);
  console.log(response.bodyUsed);
  console.log(bytes.length);
  console.log(bytes[0]);
  console.log(bytes[1]);
  console.log(bytes[2]);
  const end: any = await reader.read();
  console.log(end.done);
}
test();
`, server.URL+"/body", server.URL+"/release")
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_fetch_live_body_reader",
		source:   source,
		expected: "false\nfalse\nfalse\ntrue\n3\n97\n0\n122\ntrue\n",
	})
	if waitedForRelease.Load() {
		t.Fatal("fetch or getReader waited for body bytes before explicit read")
	}
}

func TestLinuxAMD64WinterTCFetchChunkedLiveBodyBackpressure(t *testing.T) {
	releaseFirst := make(chan struct{}, 1)
	releaseSecond := make(chan struct{}, 1)
	var waitedForFirst atomic.Bool
	var waitedForSecond atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/body":
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			select {
			case <-releaseFirst:
			case <-time.After(750 * time.Millisecond):
				waitedForFirst.Store(true)
			}
			_, _ = w.Write([]byte{97, 98, 99})
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			select {
			case <-releaseSecond:
			case <-time.After(750 * time.Millisecond):
				waitedForSecond.Store(true)
			}
			_, _ = w.Write([]byte{100, 101, 102})
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		case "/release-first":
			select {
			case releaseFirst <- struct{}{}:
			default:
			}
			_, _ = fmt.Fprint(w, "released-first")
		case "/release-second":
			select {
			case releaseSecond <- struct{}{}:
			default:
			}
			_, _ = fmt.Fprint(w, "released-second")
		}
	}))
	defer server.Close()

	source := fmt.Sprintf(`
async function test(): Promise<void> {
  const response = await fetch(%q);
  console.log(response.status);
  const stream = response.body ?? new ReadableStream();
  const reader = stream.getReader();
  await fetch(%q);
  const first: any = await reader.read();
  const firstBytes: Uint8Array = first.value;
  console.log(first.done);
  console.log(firstBytes.length);
  console.log(firstBytes[0]);
  console.log(firstBytes[1]);
  console.log(firstBytes[2]);
  await fetch(%q);
  const second: any = await reader.read();
  const secondBytes: Uint8Array = second.value;
  console.log(second.done);
  console.log(secondBytes.length);
  console.log(secondBytes[0]);
  console.log(secondBytes[1]);
  console.log(secondBytes[2]);
  const end: any = await reader.read();
  console.log(end.done);
}
test();
`, server.URL+"/body", server.URL+"/release-first", server.URL+"/release-second")
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_fetch_chunked_live_body_backpressure",
		source:   source,
		expected: "200\nfalse\n3\n97\n98\n99\nfalse\n3\n100\n101\n102\ntrue\n",
	})
	if waitedForFirst.Load() {
		t.Fatal("fetch waited for the first chunk instead of resolving after headers")
	}
	if waitedForSecond.Load() {
		t.Fatal("first reader.read waited for a later chunk instead of honoring stream backpressure")
	}
}

func TestLinuxAMD64WinterTCFetchLiveBodyReaderCancelClosesSocket(t *testing.T) {
	var observedClose atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100000")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		select {
		case <-r.Context().Done():
			observedClose.Store(true)
		case <-time.After(1 * time.Second):
		}
	}))
	defer server.Close()

	source := fmt.Sprintf(`
async function test(): Promise<void> {
  const response = await fetch(%q);
  const stream = response.body ?? new ReadableStream();
  const reader = stream.getReader();
  console.log(response.bodyUsed);
  await reader.cancel("stop");
  console.log(response.bodyUsed);
}
test();
`, server.URL)
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_fetch_live_body_reader_cancel",
		source:   source,
		expected: "false\ntrue\n",
	})
	deadline := time.Now().Add(500 * time.Millisecond)
	for !observedClose.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !observedClose.Load() {
		t.Fatal("reader.cancel did not close the live fetch socket")
	}
}

func TestLinuxAMD64WinterTCFetchNodeDifferential(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is required for WinterTC fetch differential coverage")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/echo":
			body, _ := io.ReadAll(r.Body)
			w.Header().Set("X-Server", "echo")
			payload := r.Method + ":" + string(body)
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(payload)))
			_, _ = io.WriteString(w, payload)
		case "/stream":
			w.Header().Set("Content-Length", "12")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "first-")
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			time.Sleep(40 * time.Millisecond)
			_, _ = io.WriteString(w, "second")
		case "/abort":
			time.Sleep(80 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	source := fmt.Sprintf(`
async function test(): Promise<void> {
  const headers = new Headers([["X-A", "one"], ["x-a", "two"]]);
  console.log(headers.get("x-a"));
  headers.set("x-b", " three ");
  console.log(headers.get("x-b"));

  const request = new Request(%q, { method: "POST", headers: [["X-Req", "yes"]], body: "request-body" });
  console.log(request.method);
  console.log(request.url);
  console.log(request.headers.get("x-req"));
  const requestClone = request.clone();
  console.log(await requestClone.text());
  console.log(request.bodyUsed);
  console.log(await request.text());
  console.log(request.bodyUsed);

  const response = new Response("response-body", { status: 201, statusText: "Created", headers: [["X-Res", "ok"]] });
  console.log(response.status);
  console.log(response.statusText);
  console.log(response.headers.get("x-res"));
  const responseClone = response.clone();
  console.log(await responseClone.text());
  console.log(await response.text());

  const fetched = await fetch(%q, { method: "POST", body: "wire" });
  console.log(fetched.status);
  console.log(fetched.headers.get("x-server"));
  console.log(await fetched.text());

  const streamed = await fetch(%q);
  console.log(streamed.status);
  const stream = streamed.body ?? new ReadableStream();
  const reader = stream.getReader();
  const first = await reader.read();
  console.log(first.done);
  console.log(new TextDecoder().decode(first.value));
  const second = await reader.read();
  console.log(second.done);
  console.log(new TextDecoder().decode(second.value));
  const end = await reader.read();
  console.log(end.done);

  try {
    await fetch(%q, { signal: AbortSignal.timeout(5) });
    console.log("unexpected");
  } catch (err: any) {
    console.log(err.name);
  }
}
test();
`, server.URL+"/echo", server.URL+"/echo", server.URL+"/stream", server.URL+"/abort")

	referencePath := filepath.Join(t.TempDir(), "fetch_differential.ts")
	if err := os.WriteFile(referencePath, []byte(source), 0o644); err != nil {
		t.Fatalf("write node differential fixture: %v", err)
	}
	nodeOut, err := exec.Command(node, referencePath).CombinedOutput()
	if err != nil {
		t.Fatalf("node differential reference failed: %v\nOutput:\n%s", err, string(nodeOut))
	}
	runLinuxAMD64(t, linuxAMD64Case{
		name:     "wintertc_fetch_node_differential",
		source:   source,
		expected: string(nodeOut),
	})
}
