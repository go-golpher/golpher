## Context

Golpher is at v0.0.0 (module `github.com/go-golpher/golpher`, Go 1.23.6 — pinned, no toolchain bumps in this change). The current public surface in `golpher.go`, `router.go`, `response.go`, `request.go`, `context.go`, `error.go`, `group.go`, `listen.go`, and `middleware.go` was built as a prototype and is unsafe for production:

- `App` exposes `ErrorHandler` and `Router` as exported fields and `AppConfig` is passed through verbatim; middleware and route registration mutate shared state (`app.middlewares`, `Router.routes`) without any synchronization, while `ServeHTTP` reads the same maps concurrently — a textbook data race once the server starts.
- `Response` has no committed-state tracking: `defaultErrorHandler` (error.go:27-41) renders a JSON body for unknown errors by leaking `err.Error()`, and nothing prevents a second render after the handler has already written bytes. Framework helpers also alias shared package-level `[]string` headers (`textPlainCharsetUTF8Header`, `applicationJSONHeader`, ...) into per-response `http.Header` maps via direct assignment (response.go:64, 89), so a downstream `Header().Set(...)` mutates a value shared across requests.
- Body capture is opt-out (`DisableResponseBodyCapture`) and allocates a `bytes.Buffer` per response even when no one ever calls `Response.Body()`.
- `Request.Body()` returns `*Body` and eager-reads via `io.ReadAll(request.http.Body)` (request.go:62-82); `middleware.BodyLimit` materializes the entire body up front (middleware.go:25-58), defeating any lazy contract.
- Four handler signatures coexist (`HandlerFunc`, `Handler`, `ContextHandlerFunc`, `RawHandlerFunc`) with parallel registration methods and a `Raw` shortcut that bypasses the typed pipeline.
- Route registration performs no syntax, duplicate, or conflict validation; `compileRouteSegments` accepts `:/` (empty param name) silently and two registrations of `/users/:id` vs `/users/:name` collide in the trie without warning. There is no freeze: registration after the first request silently races.
- `App.Listen` (listen.go:21-33) calls `log.Fatal` on `ListenAndServe` failure, which is untestable.
- RFC 10008 (HTTP `QUERY` method) is not supported; Go 1.23.6 `net/http` has no `MethodQuery` constant, so the package must define its own.

This design covers five capabilities (matching the proposal): `response-integrity-and-safety`, `lifecycle-hardening`, `handler-api-unification`, `request-body-safety`, and `http-query-method`. It is the bridge between `proposal.md` (why) and the per-capability specs (what) and `tasks.md` (steps).

## Goals / Non-Goals

**Goals:**

- Make the `App` lifecycle safe for concurrent use after construction: registration and freeze are synchronized, the hot path (`ServeHTTP`) takes no lock, and all mutable framework state is private and frozen before the first dispatch.
- Track committed state on `Response` for every write path (including `net/http` adapters via `ResponseController`/`Unwrap`) so the error handler is invoked exactly once and never re-renders after commit, with masked unknown errors and a single ErrorObserver hook.
- Make body capture opt-in and zero-alloc when disabled; track `Committed`/`Status`/`BytesWritten` with implicit 200 semantics.
- Collapse the handler API to a single exported `HandlerFunc` (`func(*Request, *Response) error`); remove `Handler`, `ContextHandlerFunc`, `RawHandlerFunc`, the `Raw` registration shortcut, and the `*Ctx`-returning verb variants; keep `net/http` adapters returning `HandlerFunc`.
- Make the request body lazy and bounded by default (1 MiB), with `0` meaning "use default", negative meaning "explicitly disabled", `http.MaxBytesReader` applied lazily, `Body()` returning `([]byte, error)` and caching both result and error, and `BodyLimit` middleware converted to a lazy override.
- Validate routes at registration time using RFC 9110 token syntax (no method allowlist that would block extension methods), case-sensitive methods, structural duplicate/conflict detection, terminal wildcard vs named rules, named-param uniqueness, nil-handler panic, and a hard freeze that panics on any post-`ServeHTTP` registration.
- Add first-class RFC 10008 `QUERY` support: a local `MethodQuery` constant (because Go 1.23.6 lacks it), `App.QUERY`/`Group.QUERY`, body expected and `Content-Type` required (400 if missing), handler-driven 415/`Accept-Query` per supported format, `Allow` including `QUERY`, CORS preflight and documentation coverage, and caching that includes body and metadata.

**Non-Goals:**

- Bumping Go (stays at 1.23.6) or any dependency version; adding third-party modules.
- Changing the public `AppConfig` field set beyond what the proposal lists (no new observability/tracing config, no TLS config).
- Replacing the hand-rolled trie router with a third-party router or `net/http`'s `ServeMux` pattern matching (keep the existing trie shape; only add validation + freeze).
- Redesigning `Ctx` — `Ctx` is removed entirely as a public concept once `Handler`/`ContextHandlerFunc` go away; we do not introduce a replacement context object.
- Specifying a wire format or schema for `QUERY` bodies beyond the `Content-Type`/`Accept-Query` contract; format negotiation semantics are handler-owned.
- Introducing graceful-shutdown orchestration in `App`; `Shutdown` keeps delegating to `*http.Server`.

## Decisions

### D1. App construction: AppConfig is input-only copy; config/router/error handler private and frozen

`App` keeps `AppConfig` exported **only as the input** callers pass to `New`. Internally `App` stores a private, frozen snapshot:

```go
type App struct {
    config        appConfig        // private copy, never mutated after New
    router        *Router          // private; exported access removed
    errorHandler  ErrorHandlerFunc  // private; set once in New
    errorObserver ErrorObserverFunc // private; set once in New
    middlewares   []MiddlewareFunc  // private; frozen at first ServeHTTP
    lifecycleMu   sync.Mutex        // serializes registration and the freeze transition
    frozen        atomic.Bool       // lock-free hot-path check after freeze
}
```

`appConfig` is an unexported struct mirroring the public `AppConfig` fields (after renames: `EnableResponseBodyCapture` instead of `DisableResponseBodyCapture`, plus `MaxRequestBodyBytes int64`). `New` copies the caller's `AppConfig` into `appConfig`, applies defaults, and stores it. Public field access on `App` (e.g. `app.Config`, `app.ErrorHandler`, `app.Router`) is removed; callers read `AppConfig` they themselves constructed. `App.Server` reads the private `config`.

**Why over alternatives:** keeping a mutable exported `AppConfig`/`Router`/`ErrorHandler` is exactly the current race surface. Making them private + frozen removes the race without touching the hot path. Alternative — exposing getters that copy — adds API surface and still lets callers observe mid-flight mutations; rejected. Alternative — `sync.RWMutex` on every `Handle`/`ServeHTTP` — rejected because it adds lock contention on the hot path and still allows post-start registration; freeze is cheaper and stricter.

### D2. Synchronized freeze transition with a lock-free request hot path

Registration (`Handle`, `GET`, `Use`, ...) acquires `lifecycleMu`, checks `frozen`, performs the complete mutation, and releases the mutex. The first `App.ServeHTTP` acquires the same mutex, atomically marks the app frozen, and releases it before dispatch. This creates an unambiguous boundary: a competing registration either completes before freeze or observes the frozen state and panics; no request can observe a partially built route table.

After the transition, request dispatch reads only immutable state and takes no mutex. Every registration method may use `frozen.Load()` as a fast rejection before acquiring `lifecycleMu`, but it MUST check again while holding the mutex before mutation.

**Why over alternatives:** `sync.RWMutex` with `RLock` on every request is correct but contended. An atomic flag alone has a check-then-mutate race when registration overlaps the first request. The mutex is therefore used only while building and during the one-time freeze transition; the frozen request hot path remains lock-free.

**Alternative considered:** panic in `ServeHTTP` if it observes unfrozen state and freeze there vs. an explicit `App.Freeze()` method. Rejected — the proposal mandates freeze on first `ServeHTTP`, which is friendlier than forcing callers to remember `Freeze()`.

### D3. Response tracking: Committed / Status / BytesWritten for all writes, including net/http adapters

`Response` gains:

```go
type Response struct {
    writer         http.ResponseWriter
    status         int        // 0 = implicit 200 until committed
    bytesWritten   int64
    committed      bool
    captureEnabled bool       // from app.config.EnableResponseBodyCapture
    body           bytes.Buffer
}
```

- `Committed() bool`, `Status() int` (returns `http.StatusOK` if `status == 0` and committed, else the staged or zero status), and `BytesWritten() int64` are added as accessors. Because Go cannot overload the existing chaining setter `Status(code int) *Response`, that setter is renamed to `SetStatus(code int) *Response`; all framework and consumer call sites migrate from `res.Status(code)` to `res.SetStatus(code)`.
- Every write path (`Send`, `String`, `JSON`, `JSONBytes`, `Bytes`, `XML`, `Redirect`) uses a tracking `http.ResponseWriter` that marks `committed` and records `status` before forwarding `WriteHeader`, and increments bytes after forwarding `Write`. Per-request response state is owned by the serving goroutine and does not require atomics.
- The framework **never** calls the error renderer after `committed` is true. The configured observer still receives the original error; no implicit logger is introduced.
- `http.MaxBytesReader` is applied to the request body, not the response; response writes are tracked by wrapping `writer` so that any `Write`/`WriteHeader` updates `bytesWritten` and flips `committed`. The wrapper implements `http.ResponseWriter`, provides `WriteString`/`WriteByte` with fallback to `Write`, and implements `Unwrap() http.ResponseWriter` so `http.ResponseController` can reach optional capabilities of the underlying writer.
- `Raw()` returns the wrapped writer (so callers and adapters are tracked), and `Unwrap` exposes the original `http.ResponseWriter` so `http.ResponseController(res.Raw())` reaches the real connection (flush, hijack, etc.).

**Why a wrapper over alternatives:** instrumenting each `Response` method individually misses `net/http`-adapter writes (`FromHTTPHandler`/`FromHTTPHandlerFunc` call `handler.ServeHTTP(res.writer, ...)` directly). A writer wrapper is the only way to track those without forcing adapters to change. The alternative — making `Raw()` return a non-tracked writer — defeats the goal of "all writes tracked".

**Implicit 200:** Go's `http.ResponseWriter` writes 200 on the first `Write` if `WriteHeader` was never called. The wrapper records `status = http.StatusOK` in `commit` when `status == 0` at the first byte, matching `net/http` semantics.

### D4. ErrorObserver exactly once, no render after commit

A new exported type `ErrorObserverFunc func(req *Request, res *Response, err error)` is stored privately on `App` (`app.errorObserver`, set once in `New` from `AppConfig.ErrorObserver`). `ErrorHandlerFunc` is likewise changed to `func(req *Request, res *Response, err error)`. The dispatch path calls the observer **exactly once** per request error, before invoking `app.errorHandler`. If `res.Committed()` is true, the renderer is skipped while the observer still receives the original error.

```go
// pseudocode in dispatchRequest
if err != nil {
    if app.errorObserver != nil {
        app.errorObserver(request, response, err) // exactly once
    }
    if !response.Committed() {
        app.errorHandler(request, response, err)
    }
}
```

**Why over alternatives:** letting the observer render (current pattern's risk of double-render) couples observability to transport. Firing it before the handler but skipping render when committed gives observers full visibility (including post-commit errors) while guaranteeing the wire stays clean. Alternative — firing observer after handler — loses post-commit errors if the handler swallows. Rejected.

### D5. Body capture opt-in, safe header assignment

- `AppConfig.DisableResponseBodyCapture` is **removed**. Replaced by `AppConfig.EnableResponseBodyCapture bool` (default false). `Response.captureEnabled` is set from `app.config.EnableResponseBodyCapture` at acquire time; when false, `Send`/`String`/etc. skip the `body.Write` and `Response.Body()`/`BodyString()` return `nil`/`""`.
- Capture is implemented once in `trackingResponseWriter.Write`: after the underlying writer returns, it appends only the successfully written prefix `p[:n]` when capture is enabled. Helpers and `net/http` adapters therefore share identical capture semantics, encoder-based JSON/XML require no intermediate buffer, and partial writes do not capture bytes that were not sent.
- All framework helpers stop aliasing shared `[]string` headers into per-response `http.Header`. Replace `header["Content-Type"] = contentTypeHeader(...)` with `header.Set("Content-Type", contentType)` and `header["Content-Length"] = contentLengthHeader(n)` with `header.Set("Content-Length", itoa(n))`. The shared `[]string` package vars and the `contentTypeHeader`/`contentLengthHeader` helpers are deleted (or kept as pure string-returning helpers used only to compute the value passed to `Set`). This eliminates cross-request aliasing where a downstream `Set` on a shared `[]string` would mutate state visible to other in-flight responses.

**Why over alternatives:** keeping the `[]string` aliases and "just be careful" is the current bug. `http.Header.Set` always replaces the slice entry with a fresh `[]string{value}`, so it is intrinsically safe. The minor allocation cost is negligible vs. correctness. Alternative — copying the shared slice per assignment — is what `Set` already does internally, so call `Set` directly.

### D6. Single handler signature: only HandlerFunc; adapters return HandlerFunc; Raw removed

- Exported handler types after this change: `HandlerFunc func(*Request, *Response) error` and `MiddlewareFunc func(HandlerFunc) HandlerFunc`. Everything else (`Handler`, `ContextHandlerFunc`, `RawHandlerFunc`, `Ctx`, `Context`, `adaptCtxHandler`, `adaptContextHandler`) is removed. Error callbacks receive `*Request` and `*Response` directly.
- `Router.route` loses `kind`, `ctxHandler`, `contextHandler`, `rawHandler`; only `nativeHandler HandlerFunc` + `compiledHandler HandlerFunc` remain. `routeKindRaw`/`routeKindCtx`/`routeKindContext` constants are deleted. `dispatch` and `dispatchRequest` collapse into one path.
- `App.Raw`, `App.HandleCtx`, `App.HandleContext`, `App.Get/Post/Put/Patch/Delete` (the `*Ctx`-returning variants), `App.GETContext`/... variants, and the `Router`/`Group` equivalents are removed. `Handle` is the single registration primitive; verb methods (`GET`, `POST`, `PUT`, `PATCH`, `DELETE`, plus new `QUERY`) are shorthands that call `Handle`.
- `FromHTTPHandler`/`FromHTTPHandlerFunc` remain and now return `HandlerFunc` (they already do). They do **not** bypass tracking: they call `handler.ServeHTTP(res.Raw(), req.Raw())` where `res.Raw()` is the tracking wrapper, so adapter writes are counted and flip `committed`.
- `UseHTTP` remains as a middleware adapter but is rewritten to use the same pooled `Request`/`Response`, tracking writer, and centralized error path; it MUST NOT construct a second untracked request/response pair or invoke the error renderer directly.

**Why over alternatives:** keeping `Raw` as an escape hatch preserves a tracking-bypassing path forever and doubles test surface. The tracking wrapper (`D3`) makes `Raw()` safe enough that the escape hatch is unnecessary. Alternative — keeping `ContextHandlerFunc` for "ergonomics" — was rejected by the proposal explicitly.

### D7. Body: finite default, 0 = default, negative = disabled, lazy MaxBytesReader, Body() returns ([]byte, error) and caches

- `AppConfig.MaxRequestBodyBytes int64`. Default in `New`: `1 << 20` (1 MiB). Semantics:
  - `0` → use the 1 MiB default (so a zero-value `AppConfig` is safe).
  - negative (e.g. `-1`) → explicitly disabled; the body is **not** wrapped.
  - positive → enforce that many bytes.
- Enforcement is **lazy**: `Request` stores the effective limit and a private `response *Response` reference assigned by `acquireRequest(req, response, ...)` and cleared by `releaseRequest`; it does not replace or read `http.Request.Body` during acquisition. `Body()`, body decoders, and the `net/http` adapter call one idempotent helper that wraps the original stream with `http.MaxBytesReader(r.response.Raw(), body, limit)` immediately before the first read. A declared `Content-Length` larger than the effective limit may be rejected with `413` before the handler; chunked and unknown-length bodies remain authoritatively bounded by `MaxBytesReader`.
- `Request.Body()` signature changes from `*Body` to `([]byte, error)`:
  ```go
  func (r *Request) Body() ([]byte, error)
  ```
  On first call it reads `r.http.Body` (the `MaxBytesReader`-wrapped reader), caches `bytes` and `err` in a private `bodyState` (stored on `Request`), and returns both. Subsequent calls return the cached pair. A `*http.MaxBytesError` is returned as-is so `errors.As(err, &maxBytesErr)` works; JSON/XML decoders that read `r.http.Body` directly also surface it.
- `Request.BodyBytes()` shorthand returns `([]byte, error)` (same as `Body()`). The old `*Body` type and `Body.Bytes()`/`Body.JSON()`/`Body.XML()` are removed. JSON/XML helpers move to `Request`:
  ```go
  func (r *Request) BodyJSON(v any) error   // reads+unmarshals, caches
  func (r *Request) BodyXML(v any) error
  ```
  These call `Body()` once and decode from the cached bytes.
- `middleware.BodyLimit` is rewritten as a **lazy override**: before the first read it updates the effective per-request limit; it does not read or wrap the body itself. A negative override explicitly disables the app limit for that route. The eager read in the current implementation is deleted.
- The centralized error path recognizes errors that are or wrap `*http.MaxBytesError` and renders `413 Request Entity Too Large` when the response is not committed. The observer receives the original error, preserving `errors.As` behavior.

**Why over alternatives:** eager reading (current) breaks streaming handlers and forces every request to pay the read cost. A lazy `MaxBytesReader` is the stdlib-blessed pattern. Caching the `([]byte, error)` pair preserves idempotency for handlers that call `Body()` twice and lets decoders share the cached bytes. The `0 = default` / `negative = disabled` rule makes the zero-value `AppConfig` safe while still allowing explicit opt-out — an allowlist-style "only positive is valid" would force every disabling caller to use a sentinel like `math.MaxInt64`, which is less readable.

**Alternative considered:** returning `*Body` (status quo) vs `([]byte, error)`. The proposal's wording ("`Body()` returns `([]byte, error)` and caches result/erro") fixes this; keeping `*Body` would leave the double-read ambiguity and the `.JSON()`/`.XML()` coupling that we are removing.

### D8. Route validation: RFC token syntax, case-sensitive method, structural duplicate/conflict, wildcard/named rules, nil handler, freeze panic

Validation runs in `Router.handle` (and `Group.Handle`'s underlying call), all at registration time:

1. **Method syntax (RFC 9110 token):** validate `method` as one or more ASCII `tchar` bytes. No allowlist of methods is used, so registered extension methods remain supported while empty strings, whitespace, separators, controls, and non-ASCII bytes are rejected.
2. **Method case sensitivity:** methods are compared exactly (case-sensitive). `GET` and `get` are distinct routes; this matches `net/http` and avoids the security pitfalls of case-folding. No normalization.
3. **Pattern syntax:** a single leading `/` is allowed; segments are split by `/`. A param segment is `:name` and a wildcard is `*name`; both names must match `[A-Za-z_][A-Za-z0-9_]*`. Empty parameter or wildcard names panic, duplicate parameter names within one pattern panic, and wildcards must be terminal.
4. **Nil handler:** `handler == nil` panics with `"golpher: nil handler"`.
5. **Duplicate (exact method + pattern):** re-registering the same `method + canonical pattern` panics with `"golpher: duplicate route: METHOD /path"`.
6. **Structural conflict:** registering a route whose segment shape conflicts with an existing one at the same position panics. Rules:
   - Two static segments at the same trie position are an exact-duplicate (covered by rule 5).
   - A `:param` and a `*wildcard` at the same position conflict — panic `"golpher: conflicting route: /users/:id vs /users/*name"`.
   - Two `:param` with different names at the same position conflict — panic `"golpher: conflicting route: /users/:id vs /users/:name"`. (Same name is allowed and is the same route.)
   - A static segment and a `:param`/`*wildcard` at the same position are **allowed** (static takes precedence at match time, as today).
7. **Freeze panic:** if `app.frozen.Load()` is true, any `Handle`/`Use` panics with a message such as `"golpher: app is frozen after ServeHTTP"`.

**Why over alternatives:** an allowlist (`GET/POST/PUT/PATCH/DELETE/HEAD/OPTIONS/QUERY`) would be simpler but breaks extension methods and contradicts the "no allowlist that breaks extension methods" requirement. Token validation is the RFC-correct, future-proof choice. Case-sensitivity matches `net/http` and avoids `GET`/`get` shadowing attacks. Structural-conflict detection at registration time turns today's silent trie corruption into a loud failure at boot, before any request is served.

### D9. RFC 10008 QUERY method

- `const MethodQuery = "QUERY"` exported. Local constant because Go 1.23.6's `net/http` has no `MethodQuery`. Documented as "matches the IETF RFC 10008 `QUERY` method; will alias to `http.MethodQuery` if/when Go adds it (no behavior change)".
- `App.QUERY(pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc)` and `Group.QUERY(...)` are added as shorthands calling `Handle(MethodQuery, ...)`. `Router` dispatch treats `QUERY` like any other method (D8's token validator accepts it).
- **Content contract:** RFC 10008 gives QUERY request content semantics and requires the server to reject a missing or content-inconsistent `Content-Type`. The dispatch layer rejects missing `Content-Type` with `400 Bad Request` before the handler. It does not reject solely on `Content-Length: 0`, because whether empty content is valid depends on the declared media type; decoders or handlers reject malformed/inconsistent content with `400`, unsupported types with `415`, and semantically unprocessable queries with `422`.
- **Format negotiation (handler-owned):** the framework does not mandate a query format. Each resource validates `Content-Type`, returning `415` for unsupported media types and optionally setting a correctly formatted RFC 10008 `Accept-Query` structured field. No convenience formatter is added in this change because emitting Structured Fields correctly is separate API work.
- **`Allow` header:** `Router.allowedMethods` already enumerates all methods matching a path; with `QUERY` registered, it is included automatically (no special-casing). `OPTIONS` auto-handling, if added later, will include `QUERY` for free.
- **CORS preflight:** the framework does not implement CORS itself (out of scope, stays handler/middleware owned), but the documentation and examples must show registering `QUERY` in any `Access-Control-Request-Method` allowlist and the `OPTIONS` preflight handler echoing `QUERY`. This is called out in specs and README.
- **Caching:** the documentation/specs must state that responses to `QUERY` are cacheable only if the cache key includes the request body and the `Content-Type` (and any `Vary`-selected headers). RFC 10008 explicitly warns that GET-style caching keyed only on the URI is incorrect for `QUERY`. This is a documentation/spec requirement, not framework code, since golpher does not ship a cache.
- **Routing tests:** `golpher_test.go` adds `QUERY` alongside `GET`/`POST`/... in dispatch, `Allow`, and conflict tests.

**Why over alternatives:**
- *Local constant vs. `net/http` constant:* Go 1.23.6 has none; using a local const now and aliasing later is the lowest-friction path. Alternative — importing a third-party `httpquery` package — violates "no new dependencies".
- *Framework-enforced Content-Type vs. handler-enforced content validity:* missing `Content-Type` is universally rejected by RFC 10008. Empty or malformed content and supported media types depend on the target resource, so those checks remain with the decoder or handler.
- *Caching as docs vs. framework code:* golpher has no cache, so docs/specs are the only lever. Alternative — adding a `Vary: Content-Type` automatically on `QUERY` responses — was considered and rejected because `Vary` semantics depend on what the handler negotiates; auto-setting it could break legitimate non-cacheable handlers.

## Risks / Trade-offs

- **[Breaking change: `App.Listen` returns `error`]** → Migration: callers that wrote `app.Listen()` must now `if err := app.Listen(); err != nil { /* handle */ }`. The `log.Fatal` behavior is intentionally not preserved; tests and graceful startups need the error. Document in README and ROADMAP.
- **[Breaking change: handler API collapse]** → Migration: every `Handler`/`ContextHandlerFunc`/`RawHandlerFunc` call site must convert to `HandlerFunc`. `RawHandlerFunc` users migrate to `FromHTTPHandlerFunc`. Provide a `golpher migrate`-style codemod doc in `docs/` (no automated tooling; just patterns). Risk: silent semantic drift if a user's `Handler` used `Ctx` state that has no equivalent on `Request`/`Response`. Mitigation: `Ctx` was a thin wrapper over `Request`/`Response`, so the migration is mechanical.
- **[Breaking change: `EnableResponseBodyCapture` inverts default]** → Migration: any code setting `DisableResponseBodyCapture: true` (opt-out) now gets the new default (capture off) for free; code that *relied* on capture being on by default must add `EnableResponseBodyCapture: true`. Risk: middleware/tests reading `Response.Body()` silently get `nil`. Mitigation: spec scenario + README callout + test coverage.
- **[Breaking change: `Request.Body()` signature change]** → Migration: every `req.Body().Bytes()`/`.JSON(v)`/`.XML(v)` call becomes `req.Body()` / `req.BodyJSON(v)` / `req.BodyXML(v)`. Risk: compile errors are loud (good), but `req.Body().Bytes()` returning `nil` on capture-off was already a footgun. Mitigation: new API returns `([]byte, error)` so misuse is a compile error, not a silent nil.
- **[Risk: freeze panic in long-running apps that lazily register routes]** → Mitigation: the contract is "register before serving"; this matches every major Go router (`chi`, `httprouter`, `gorilla/mux` post-`Lock()`). Document loudly. Provide `App.IsFrozen() bool` for callers that want to guard.
- **[Risk: tracking wrapper hides `http.Hijacker`/`http.Flusher` if not implemented]** → Mitigation: the wrapper implements `io.StringWriter`, `io.ByteWriter` only when the underlying writer does, and **always** implements `http.ResponseWriter` + `Unwrap()`. `http.ResponseController(res.Raw())` reaches the real writer for `Flush`/`Hijack`. Document that callers should use `http.ResponseController` rather than type-asserting `res.Raw()`.
- **[Risk: `MaxBytesReader` lazy wrap changes error type for handlers that read the body directly]** → Mitigation: `*http.MaxBytesError` is preserved through `errors.As`. Spec scenario asserts this. Document that handlers must use `errors.As` rather than string-matching.
- **[Risk: structural conflict detection rejects patterns that today silently coexist]** → Mitigation: today those patterns silently corrupt the trie (one shadows the other). Loud panic at boot is strictly better. Document the rules in `docs/routing.md`.
- **[Risk: `QUERY` 400 for missing Content-Type breaks permissive clients]** → Mitigation: RFC 10008 explicitly requires rejection when media type information is missing; document the requirement and provide examples.
- **[Risk: `Accept-Query` is advisory; caches may ignore it]** → Mitigation: out of scope for the framework; documented in specs. Handlers that must prevent caching set `Cache-Control: no-store`.
- **[Trade-off: lock-free freeze vs. mid-flight `Use` after first request]** → Accepted: `Use` after the first `ServeHTTP` panics. This is stricter than a mutex but matches the proposal.
- **[Trade-off: `0 = default` body limit vs. "0 = unlimited" convention]** → Chosen because zero-value `AppConfig` must be safe-by-default; an unlimited zero-value would reintroduce the unbounded-body risk this change fixes. Explicit unlimited is `MaxRequestBodyBytes: -1`.

## Migration Plan

1. **Update call sites (mechanical, per file):**
   - Replace response status chaining `res.Status(code)` with `res.SetStatus(code)`; use the new zero-argument `res.Status()` only to inspect the effective status.
   - `app.Listen()` → `if err := app.Listen(); err != nil { ... }` everywhere (search for `.Listen(`).
   - Replace `app.Get/Post/Put/Patch/Delete` (the `*Ctx` variants) and `app.*Context` variants with `app.GET/POST/...` + a `HandlerFunc`. Replace `app.Raw` with `app.Handle(method, pattern, FromHTTPHandlerFunc(fn))`. Replace `app.HandleCtx`/`app.HandleContext` with `app.Handle`.
   - Replace `DisableResponseBodyCapture: true` with `EnableResponseBodyCapture: false` (or just remove it, since false is the new default). Replace `DisableResponseBodyCapture: false` (i.e. capture on) with `EnableResponseBodyCapture: true`.
   - Replace `req.Body().Bytes()` with `b, err := req.Body()`. Replace `req.Body().JSON(&v)` with `err := req.BodyJSON(&v)`. Same for XML.
   - Replace `ctx := req.acquireCtx(res); route.ctxHandler(ctx)`-style code (none outside the framework, but any user that imported `Handler`/`ContextHandlerFunc`) with direct `HandlerFunc` bodies.
2. **Add `QUERY` routes** where desired: `app.QUERY("/search", ...)` / `group.QUERY(...)`.
3. **Opt into body capture** in tests/middleware that read `Response.Body()`: `AppConfig{EnableResponseBodyCapture: true}`.
4. **Tune body limit** if the 1 MiB default is wrong: `AppConfig{MaxRequestBodyBytes: 10 << 20}` for 10 MiB, or `-1` to disable.
5. **Hook the ErrorObserver** (optional): `AppConfig{ErrorObserver: func(req, res, err) { logger.Log(req, err) }}`.
6. **Run tests**, expecting: tests asserting on unknown-error body text must change to expect the generic `500 Internal Server Error` message; tests registering routes after the first `ServeHTTP` must expect a panic; tests for `/users/:id` + `/users/:name` must expect a registration panic.
7. **Update docs**: README examples, `docs/routing.md` (validation rules), `docs/query.md` (new, RFC 10008 + caching notes), ROADMAP entries referencing removed types.
8. **Rollback strategy:** the change is a single PR (no staged rollout for a v0.0.0 library). If a downstream prototype breaks, the previous commit is the rollback. No schema/data migration is involved.

## Open Questions

None.
