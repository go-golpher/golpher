# Request Body Safety Specification

## Purpose

Define configurable maximum request body size with default (1 MiB), zero-means-default, and negative-means-unlimited semantics. Apply the limit lazily via `http.MaxBytesReader` upon first body read. Expose `Request.Body() ([]byte, error)` with caching. Provide `BodyLimit` middleware that overrides the per-route limit. Ensure the body is never eagerly read if unused and that body-limit errors render as `413 Request Entity Too Large`.

## Requirements

### Requirement: BODY-001 — Configurable MaxRequestBodyBytes with documented semantics

`AppConfig` SHALL expose a `MaxRequestBodyBytes int64` field. The default value SHALL be `1 << 20` (1 MiB). A value of `0` SHALL mean "use the default" (1 MiB). A negative value SHALL mean "no limit" (unlimited). The limit SHALL be applied lazily via `http.MaxBytesReader` when the body is first read.

#### Scenario: Default limit is 1 MiB
**WHEN** `App` is created with default configuration
**THEN** the effective maximum request body size SHALL be `1048576` bytes

#### Scenario: Zero means default
**WHEN** `MaxRequestBodyBytes` is explicitly set to `0`
**THEN** the effective limit SHALL be `1048576` bytes (same as default)

#### Scenario: Negative means unlimited
**WHEN** `MaxRequestBodyBytes` is set to `-1`
**THEN** the framework SHALL NOT apply any body size limit
**AND** the request body SHALL NOT be wrapped with `MaxBytesReader`

#### Scenario: Custom positive limit applied
**WHEN** `MaxRequestBodyBytes` is set to `65536` (64 KiB)
**THEN** requests with body larger than 64 KiB SHALL fail with a size-limit error

---

### Requirement: BODY-002 — Lazy MaxBytesReader wrapping

The request body `io.ReadCloser` SHALL be wrapped with `http.MaxBytesReader` only when a handler or decoder first attempts to read the body. No eager materialization SHALL occur during request acquisition or middleware pre-processing.

#### Scenario: Body not read — MaxBytesReader not constructed
**WHEN** a handler does not read the request body at all
**THEN** the `http.Request.Body` SHALL NOT be wrapped with `*http.MaxBytesReader`
**AND** no body bytes SHALL be read from the connection

#### Scenario: Body read triggers lazy wrapper
**WHEN** a handler calls `request.Body()` for the first time
**THEN** `http.MaxBytesReader` SHALL be applied to the original body
**AND** reading SHALL start from the wrapped reader

#### Scenario: Body limit enforced on read
**WHEN** a handler reads a body that exceeds the configured limit
**THEN** the read SHALL return an error
**AND** that error SHALL be a `*http.MaxBytesError` discoverable with `errors.As`

---

### Requirement: BODY-003 — Body() returns ([]byte, error) with cache

`Request.Body()` SHALL return `([]byte, error)`. The result SHALL be cached so that repeated calls return the same data without re-reading. The `Body` helper struct and its `Bytes()`, `JSON()`, `XML()` methods SHALL be removed from the public API.

#### Scenario: Body() returns bytes and error directly
**WHEN** a handler calls `data, err := request.Body()`
**THEN** `data` SHALL contain the full body bytes on success
**AND** `err` SHALL be nil on success

#### Scenario: Body() returns size-limit error via errors.As
**WHEN** the body exceeds the configured limit
**THEN** the error from `Body()` SHALL support `errors.As(err, &maxBytesErr)` where `maxBytesErr` is `*http.MaxBytesError`

#### Scenario: Body() result is cached
**WHEN** `request.Body()` is called twice
**THEN** the second call SHALL return the same bytes without reading from the underlying stream again

#### Scenario: Body() after error returns same error
**WHEN** the first call to `request.Body()` returns a read error
**THEN** the second call SHALL return the same error without re-reading

---

### Requirement: BODY-004 — BodyLimit middleware uses lazy delegate

`BodyLimit` middleware SHALL be refactored to use `http.MaxBytesReader` wrapping instead of eager `io.ReadAll`. If the application or route-level `MaxRequestBodyBytes` is set, the middleware SHALL override the limit for the wrapped reader.

#### Scenario: BodyLimit overrides the app limit
**WHEN** the app-level `MaxRequestBodyBytes` is 1 MiB
**AND** `BodyLimit(64*1024)` middleware is applied to a route
**THEN** the route SHALL enforce a 64 KiB limit
**AND** requests exceeding 64 KiB SHALL receive `413 Request Entity Too Large`

#### Scenario: BodyLimit negative means unlimited for that route
**WHEN** `BodyLimit(-1)` middleware is applied
**THEN** the route SHALL have no body size limit regardless of the app-level setting

#### Scenario: BodyLimit zero means use app default
**WHEN** `BodyLimit(0)` middleware is applied
**THEN** the route SHALL use the app-level `MaxRequestBodyBytes` setting

---

### Requirement: BODY-005 — Body is not read if unused

If a handler never calls any method that triggers body reading, the `Request.Body` SHALL remain untouched. Specifically, the `Request` struct SHALL NOT eagerly read the body during request acquisition or at any point before the first explicit read call.

#### Scenario: Unread body leaves original stream intact
**WHEN** a handler returns without calling `Body()`, `Bind()`, or any body-reading decoder
**THEN** the original `http.Request.Body` stream SHALL NOT have been read from
**AND** no `MaxBytesReader` wrapper SHALL have been applied

#### Scenario: Body is not read in middleware chains unless explicitly requested
**WHEN** middleware does not read the body
**AND** the handler does not read the body
**THEN** the body stream SHALL remain unread for the entire request lifecycle

---

### Requirement: BODY-006 — Body limit errors render as 413

When a handler returns an error that is or wraps `*http.MaxBytesError`, the framework SHALL render `413 Request Entity Too Large` if the response is not committed. The configured observer SHALL receive the original error.

#### Scenario: Returned MaxBytesError becomes 413
**WHEN** reading the bounded body returns `*http.MaxBytesError`
**AND** the handler returns that error before committing the response
**THEN** the framework SHALL respond with status `413 Request Entity Too Large`

#### Scenario: Observer receives original size error
**WHEN** a configured observer handles a body overflow error
**THEN** `errors.As` on the observed error SHALL discover `*http.MaxBytesError`
