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
3. **Encoding and URL**: `TextEncoder` / `TextDecoder` and the byte-view foundation are implemented; `URL` and `URLSearchParams` are implemented with full static methods (`canParse`, `parse`), component setters, normalization, and live two-way synchronization; `URLPattern` is implemented with component extraction, pattern parsing (literals, wildcards, named parameters, regex groups/alternations), `test()`, `exec()`, and differential conformance; `TextEncoderStream` and `TextDecoderStream` are implemented on top of `TransformStream`.
4. **Binary/body types**: `Blob`, `File`, `FormData` and Body-compatible byte/string conversion are implemented with parts concatenation, MIME normalization, File inheritance, FormData key/value storage, Blob-to-File wrapping, async extraction methods (`text()`, `arrayBuffer()`, `bytes()`), and `Blob.prototype.stream()` ReadableStream backing.
5. **Streams**: `ByteLengthQueuingStrategy` and `CountQueuingStrategy`; `ReadableStream` (`start`, `pull`, `cancel`, `enqueue`, `close`, `getReader`, `pipeThrough`, `pipeTo`, `tee`, `from`); `ReadableStreamDefaultReader` (`read`, `releaseLock`, `cancel`); `ReadableStreamDefaultController`; `WritableStream` (`start`, `write`, `close`, `abort`, `getWriter`); `WritableStreamDefaultWriter` (`write`, `close`, `abort`, `releaseLock`); `WritableStreamDefaultController`; `TransformStream` (`readable`, `writable`, `transform`, `flush`) and `TransformStreamDefaultController` according to ECMA-429.
6. **Fetch**: `Headers`, `Request`, `Response`, `fetch`, cancellation, redirects, streaming bodies and default `User-Agent` behavior.
7. **Crypto/performance/compression**: `crypto`, `Crypto`, `CryptoKey`, `SubtleCrypto`, `performance`, compression/decompression streams.
8. **Messaging and WebAssembly**: `MessageChannel`, `MessagePort`, Promise rejection events and the required WebAssembly JS/Web APIs.
9. **Conformance closeout**: WinterTC matrix at 100%, full native suite, differential tests and documented server-runtime deviations.

Web Workers themselves are not required by ECMA-429; worker-global additions apply only if ts-pro later exposes a WorkerGlobalScope-equivalent environment.

## Foundation blocker matrix

WinterTC completion is gated by shared runtime/compiler foundations rather than by API count alone. These are completed before broad surface expansion so later phases reuse one correct ABI instead of duplicating fragile representations.

| Foundation | Why it blocks | Completion gate |
| --- | --- | --- |
| GC-safe `any` / JSValue cells | WebIDL `any`, reasons, chunks and bodies cannot live in raw reference slots | one shared JSValue cell/storage ABI, GC stress coverage |
| Callback / closure ABI | Event, Streams and Fetch invoke arbitrary user callbacks that clobber caller registers | callback values reloaded/materialized across calls; mutation/order e2e |
| WebIDL dictionaries/coercion | Constructors repeatedly need optional members/defaults/string/boolean conversion | shared dictionary read/default helpers and error coercion tests |
| Web async jobs | rejection hooks, streams and fetch depend on deterministic microtask semantics | promise-job + rejection event ordering e2e |
| Byte buffer primitive | Encoding, Blob, Streams, Fetch, Crypto and Compression all exchange bytes | GC-safe byte storage plus slice/copy/string conversion tests |
| Native capability layer | Fetch, Crypto and Compression require OS/runtime services | explicit network/random/hash/compression modules with local integration tests |

The active implementation order is: JSValue storage -> callback/dictionary foundations -> byte buffers -> async hooks -> native capabilities -> Phase 2 through Phase 9 conformance closeout.
