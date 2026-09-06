# WinterTC Minimum Common Web API

## Target

`ts-pro` targets the WinterTC Minimum Common Web API draft dated **31 July 2026** (ECMA-429, 2025 snapshot) as a Web-interoperability surface.

This is **not** a Cloudflare Workers compatibility target. Runtime/vendor extensions are out of scope unless independently required by ts-pro.

Conformance is only claimed for APIs whose behavior is implemented according to the normative W3C/WHATWG specification referenced by WinterTC. Merely exposing a global name does not count.

## Completion gates

- Every supported interface/global has semantic typing, lowering/runtime behavior, and native e2e coverage.
- Observable behavior is differential-tested against a conforming reference runtime where the server-runtime semantics are comparable.
- Error names/types and coercions are tested, not just happy paths.
- APIs with network/crypto/streaming behavior have deterministic local integration tests.
- `docs/WINTERTC.md` is the source-of-truth implementation matrix.

## Implementation phases

1. **Global foundation**: `globalThis`, `self`, `atob`, `btoa`, timers, `queueMicrotask`, error/rejection hooks, `structuredClone`, console completeness.
2. **DOM foundation**: `DOMException`, `Event`, `CustomEvent`, `ErrorEvent`, `MessageEvent`, `EventTarget`, `AbortController`, `AbortSignal`.
3. **Encoding and URL**: `TextEncoder`, `TextDecoder`, encoder/decoder streams, `URL`, `URLSearchParams`, `URLPattern`.
4. **Binary/body types**: `Blob`, `File`, `FormData` and Body-compatible byte/string conversion.
5. **Streams**: readable/writable/transform streams, readers/controllers/writers and queuing strategies required by ECMA-429.
6. **Fetch**: `Headers`, `Request`, `Response`, `fetch`, cancellation, redirects, streaming bodies and default `User-Agent` behavior.
7. **Crypto/performance/compression**: `crypto`, `Crypto`, `CryptoKey`, `SubtleCrypto`, `performance`, compression/decompression streams.
8. **Messaging and WebAssembly**: `MessageChannel`, `MessagePort`, Promise rejection events and the required WebAssembly JS/Web APIs.
9. **Conformance closeout**: WinterTC matrix at 100%, full native suite, differential tests and documented server-runtime deviations.

Web Workers themselves are not required by ECMA-429; worker-global additions apply only if ts-pro later exposes a WorkerGlobalScope-equivalent environment.
