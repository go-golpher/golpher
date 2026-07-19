# Response Integrity and Safety Specification

## Purpose

Define safe error response behavior: unknown errors are masked as generic `500 Internal Server Error` while preserving the original error for observers; `ErrorGolpher` errors preserve their message. Provide an `ErrorObserver` hook invoked exactly once per error. Expose response committed state, status code, and bytes written. Support opt-in response body capture via `EnableResponseBodyCapture`. Ensure all framework helper methods use safe header mutation (`Header.Set`) to prevent cross-request aliasing through shared slice literals.

## Requirements

### Requirement: RES-INT-001 — Unknown error must be masked in default error handler

The default error handler SHALL NOT leak the original error message in the HTTP response body for errors that are not `ErrorGolpher`. The original error SHALL be available only through a configured observer or logger.

#### Scenario: Unknown error produces generic 500 body
**WHEN** a handler returns an `error` that is not `ErrorGolpher` (e.g. `errors.New("database connection refused")`)
**THEN** the HTTP response SHALL have status `500`
**AND** the response body SHALL contain the message `Internal Server Error`
**AND** the response body SHALL NOT contain the string `"database connection refused"`

#### Scenario: ErrorGolpher error preserves its message
**WHEN** a handler returns `ErrorGolpher{Code: 404, Message: "not found"}`
**THEN** the default error handler SHALL serialize the full `ErrorGolpher` including `"message": "not found"`

#### Scenario: Observer receives the original cause for unknown errors
**WHEN** an `ErrorObserver` is configured on the app
**AND** a handler returns an unknown error `errors.New("db timeout")`
**THEN** the observer SHALL be called with the original `"db timeout"` error before the error handler runs
**AND** the response body SHALL remain generic (`Internal Server Error`)

---

### Requirement: RES-INT-002 — ErrorObserver receives every framework error exactly once

The framework SHALL support an optional `ErrorObserverFunc func(*Request, *Response, error)` set through `AppConfig`. It SHALL be invoked for every request-processing error, including errors returned after the response is committed. The observer MUST NOT own response rendering.

#### Scenario: Observer invoked before error handler
**WHEN** a route handler returns an error
**THEN** the `ErrorObserver` function SHALL be called with the error, the `*Request`, and the `*Response`
**AND** the error handler SHALL run after the observer returns

#### Scenario: Observer is called exactly once per error
**WHEN** a single error occurs during request processing
**THEN** the observer SHALL be invoked exactly once for that error

#### Scenario: Observer is nil by default — no-op
**WHEN** `AppConfig.ErrorObserver` is not set (nil)
**THEN** the error handler SHALL run without calling any observer

---

### Requirement: RES-INT-003 — Response exposes committed state, status, and bytes written

`Response` SHALL expose `Committed() bool`, `Status() int`, and `BytesWritten() int64`. The chaining status setter SHALL be named `SetStatus(code int) *Response` so it does not collide with the zero-argument getter. Once committed, any attempt to write the status again MUST NOT emit a second `WriteHeader` call; body writes SHALL remain possible for normal streaming. The framework error handler MUST NOT append a second error response body.

#### Scenario: Committed status prevents second WriteHeader
**WHEN** a handler commits status 200 by writing a response and then returns an error
**THEN** a second status MUST NOT be written to the wire
**AND** the `Response` SHALL report the original committed status via a public method

#### Scenario: Status setter and getter have distinct names
**WHEN** a handler calls `response.SetStatus(201)` before writing
**THEN** `response.Status()` SHALL return `201`
**AND** the API SHALL NOT declare two methods named `Status`

#### Scenario: Bytes written counter increments
**WHEN** a handler writes body bytes via `Send` or `String`
**THEN** the `Response` SHALL report the total number of bytes written

#### Scenario: Error handler does not write after commit
**WHEN** a handler has already committed the response (status + partial body)
**AND** the handler subsequently returns an error
**THEN** the framework error handler SHALL NOT write an additional error response body
**AND** the `Response` SHALL remain in committed state

#### Scenario: Body writes succeed after commit
**WHEN** the response is already committed
**AND** a handler calls `Send` or `String`
**THEN** the body data SHALL still be written to the wire
**AND** the bytes written counter SHALL be updated

---

### Requirement: RES-INT-004 — Body capture opt-in via EnableResponseBodyCapture

`AppConfig.DisableResponseBodyCapture` SHALL be removed and replaced with `EnableResponseBodyCapture bool`. When false (default), the `Response` MUST NOT allocate or populate the body capture buffer. When true, `Response.Body()` and `Response.BodyString()` SHALL return the captured bytes.

#### Scenario: Body capture off by default
**WHEN** `AppConfig` is created with default values
**THEN** `EnableResponseBodyCapture` SHALL be `false`
**AND** calls to `Body()` or `BodyString()` on a `Response` SHALL return `nil` / empty

#### Scenario: Body capture on returns captured bytes
**WHEN** `EnableResponseBodyCapture` is set to `true`
**AND** a handler calls `response.String("hello")`
**THEN** `response.BodyString()` SHALL return `"hello"`

#### Scenario: Capture covers every response helper
**WHEN** capture is enabled
**AND** a handler writes through `Send`, `String`, `JSON`, `JSONBytes`, `Bytes`, or `XML`
**THEN** `Response.Body()` SHALL contain the bytes written by that helper

#### Scenario: Capture stores only successfully written bytes
**WHEN** the underlying writer accepts only a prefix and returns a short write
**THEN** capture SHALL retain only the accepted prefix
**AND** `BytesWritten()` SHALL equal the accepted byte count

#### Scenario: Body capture off does not allocate buffer
**WHEN** `EnableResponseBodyCapture` is `false`
**AND** a handler writes 1 MiB of data via `Send`
**THEN** the `Response` SHALL NOT retain a copy of the data in memory

---

### Requirement: RES-INT-005 — Framework helpers use Header.Set, not shared slice literals

All framework helper methods (`String`, `Bytes`, `JSON`, `XML`, `Redirect`, and the error handler) SHALL use `http.Header.Set(key, value)` or equivalent safe mutation instead of assigning package-level `[]string` values directly via map index assignment. This eliminates mutable cross-request aliases through shared backing arrays.

#### Scenario: Header.Set used for content type
**WHEN** a handler calls `response.String("ok")`
**THEN** the `Content-Type` header SHALL be set via `Header().Set("Content-Type", ...)` or equivalent safe API
**AND** concurrent requests SHALL NOT observe each other's header mutations

#### Scenario: Content-Length header uses Set
**WHEN** a handler calls `response.Bytes(200, "application/json", data)`
**THEN** the `Content-Length` header SHALL be set via a safe method that does not alias a shared `[]string`

#### Scenario: Pre-allocated shared slices removed
**WHEN** the package initializes
**THEN** no package-level `[]string` variables SHALL be used as direct values for `http.Header` map entries
**AND** only `Header().Set()` or individual string assignment SHALL be used for header writes
