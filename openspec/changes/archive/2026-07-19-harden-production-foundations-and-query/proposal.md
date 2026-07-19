# Harden Production Foundations and Query

## Why

Golpher is at v0.0.0 with a wide handler API surface (four handler signatures), eager body reading, unconditional error message leakage, mutable response headers after write, and a `Listen` that calls `log.Fatal` on error. These choices are acceptable for a prototype but unsafe for production. At the same time, RFC 10008 defines the HTTP `QUERY` method for safe read-only queries with a request body — a natural fit for Golpher's typed-Handler design. This change hardens the core surface and adds first-class `QUERY` support before the v1 API is committed.

## What Changes

### 1. Security and response integrity

- **Mask unknown errors**: `defaultErrorHandler` currently leaks `err.Error()` as the response body for non-`ErrorGolpher` errors. After this change, unknown errors produce a generic `500 Internal Server Error` message. The original error is available only to a configured observer or logger.
- **ErrorObserver**: Introduce an optional, application-scoped observer that receives every framework error with request and response context before the `ErrorHandler` runs, without owning response rendering.
- **Committed state on Response**: `Response` exposes whether the response has started, the effective status, and bytes written. Once committed, status changes cannot emit another header; body writes remain possible for normal streaming, but the framework error handler never appends a second error response.
- **Body capture opt-in**: `Response` body capture (for `Response.Body()` / `BodyString()`) becomes opt-in via `AppConfig.EnableResponseBodyCapture` instead of opt-out via `DisableResponseBodyCapture`. This removes the per-response buffer allocation for the majority of production handlers that never read the captured body.
- **Header.Set**: Framework helpers use `http.Header.Set` instead of assigning package-level shared `[]string` values into per-response headers, eliminating mutable cross-request aliases.

### 2. Lifecycle and routing validation

- **`Listen` returns `error`**: `App.Listen(...)` currently calls `log.Fatal` when `ListenAndServe` returns an error other than `http.ErrServerClosed`. This makes it impossible to handle startup failures in tests or graceful startup sequences. `Listen` will return `error` and the caller decides what to do.
- **Freeze on first `ServeHTTP`**: The first call to `App.ServeHTTP` freezes the private route table. Any subsequent route or middleware registration panics with a clear message. This eliminates data races between registration and concurrent requests.
- **Duplicate, conflicting, and invalid route panics**:
  - Registering the exact same method + pattern twice panics at registration time.
  - Registering a route that conflicts with an existing dynamic route (e.g. `/users/:id` after `/users/:name`) panics.
  - Patterns with invalid syntax (e.g. empty param name `:/`) panic with a descriptive message.

### 3. Handler API unification (breaking)

- **Single handler signature**: Only `HandlerFunc` (`func(*Request, *Response) error`) is exported. The following are removed:
  - `Handler` (the `*Ctx`-based signature)
  - `ContextHandlerFunc`
  - `RawHandlerFunc`
  - `Middleware` aliases/types that only exist to support these signatures.
- **Unified error callbacks**: `ErrorHandlerFunc` and `ErrorObserverFunc` receive `*Request`, `*Response`, and `error` directly; the redundant exported `Context` and `Ctx` wrappers are removed.
- **Removed convenience methods**: `HandleCtx`, `HandleContext`, `Raw`, `Get`, `Post`, `Put`, `Patch`, `Delete` (the `*Ctx`-returning variants), `GETContext`, `POSTContext`, `PUTContext`, `PATCHContext`, `DELETEContext` on `App`, and their equivalents on `Router`, are removed.
- **`Handle` becomes the single registration method** on `App` and `Group`. Verb methods (`GET`, `POST`, etc.) remain as shorthand that calls `Handle`.
- **Adapter functions** (`FromHTTPHandler`, `FromHTTPHandlerFunc`) remain unchanged since they target the `net/http` boundary.

### 4. Request body safety

- **Configurable safe limit via `AppConfig`**: A new application setting establishes a finite default maximum request body size and permits explicit override for applications with different payload requirements.
- **`MaxBytesReader` lazy enforcement**: The request body is wrapped with `http.MaxBytesReader` without eager middleware materialization. Bytes are read only when a handler or decoder asks for them.
- **Explicit read error**: The body API returns read and size-limit errors directly, preserving `*http.MaxBytesError` for `errors.As`; JSON and XML decoding propagate those errors.

### 5. First-class HTTP QUERY method (RFC 10008)

- **`MethodQuery` constant**: `const MethodQuery = "QUERY"` exported from the package.
- **`App.QUERY` and `Group.QUERY`**: Shorthand methods that register a handler for the `QUERY` HTTP method, following the same signature and middleware chain as `GET`/`POST`.
- **`Router` support**: The router treats `QUERY` as a first-class method in its internal dispatch and `Allow` header generation, exactly like `GET` or `POST`.
- **Routing tests** include `QUERY` alongside existing methods.
- **Naming clarity**: The method is called `QUERY` (uppercase, matching Go's `http.MethodGet` style) and is never confused with URI query parameters (accessed via `Request.Query(name)`).

## Capabilities

### New Capabilities

Each capability below will get its own delta spec in this change.

| Capability ID | Description |
|---|---|
| `response-integrity-and-safety` | ErrorObserver, committed state tracking, no error rewrite after commit, opt-in body capture, safe header assignment, unknown error masking |
| `lifecycle-hardening` | Listen returns error, freeze on first ServeHTTP, duplicate/conflicting/invalid route panics |
| `handler-api-unification` | Single `func(*Request, *Response) error` handler signature; remove `Handler`, `ContextHandlerFunc`, `RawHandlerFunc` and their registration methods |
| `request-body-safety` | Configurable `MaxRequestBodyBytes`, `MaxBytesReader` lazy loading, explicit size-limit errors |
| `http-query-method` | `MethodQuery` constant, `App.QUERY`, `Group.QUERY`, Router dispatch support |

### Modified Capabilities

(No existing specs to modify — section intentionally empty.)

## Impact

### Breaking changes

1. `App.Listen` signature changes from returning nothing to returning `error`. Callers that rely on `log.Fatal` must handle the error.
2. `Handler`, `ContextHandlerFunc`, `RawHandlerFunc` types and their registration methods are removed. Code using `App.Get`, `App.Post`, etc. with `Handler` args must migrate to `HandlerFunc`.
3. The redundant `Ctx` and `Context` wrappers are removed; error callbacks now receive `*Request`, `*Response`, and `error` directly.
4. Mutable `App.Config`, `App.Router`, and `App.ErrorHandler` fields are removed; configuration is supplied through `AppConfig` and copied by `New`.
5. The response status setter is renamed from `Status(code)` to `SetStatus(code)` so `Status()` can report the effective status.
6. `Request.Body()` changes from returning `*Body` to returning `([]byte, error)`; body decoding helpers move to `Request`.
7. `AppConfig.DisableResponseBodyCapture` is removed and replaced by `EnableResponseBodyCapture`; capture is disabled by default.
8. Route and middleware registration after the first `ServeHTTP` call panics, and invalid, duplicate, or conflicting routes now panic during registration.
9. Default error handling no longer exposes unknown error messages and maps body-limit errors to `413`.
10. The default finite body limit can reject requests that previously read without a framework limit; callers must explicitly configure a larger value or a negative value to disable it.

### Public files affected

- `golpher.go` — `App` struct, `New`, handler methods, `Use`
- `listen.go` — `Listen`, `Serve`
- `router.go` — `Router.ServeHTTP`, route registration, `acquireRequest`
- `response.go` — `Response` struct, committed state, body capture, `Header`
- `request.go` — `Request.Body()`, limit wiring
- `error.go` — `ErrorHandler`, `defaultErrorHandler`
- `context.go` — remove `Handler`, `ContextHandlerFunc`, `RawHandlerFunc`, adapters
- `group.go` — `Group.Handle`, add `QUERY`
- `middleware.go` — adjust for new body semantics if needed

### Documentation and tests

- All examples and ROADMAP references to removed handler types must be updated.
- Tests must cover committed-state behavior, QUERY routing, route conflicts, body limit exceeded, error masking, and ErrorObserver calls.
- `README.md` and `docs/` must reflect the unified handler API.

### Dependencies and toolchain

- No new third-party dependencies.
- No dependency or Go version changes.
