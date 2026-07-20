# Lifecycle Hardening Specification

## Purpose

Define safe application lifecycle semantics: `App.Listen` returns `error` instead of terminating the process. The route table freezes atomically on the first `ServeHTTP` call; any registration after freeze panics. Runtime configuration and router state are private and immutable after construction. Duplicate routes, conflicting dynamic parameters, invalid pattern syntax, nil handlers, and invalid HTTP methods all panic at registration with clear diagnostic messages. Static trailing-slash aliases are registered automatically.

## Requirements

### Requirement: LIF-001 — Listen returns error instead of calling log.Fatal

`App.Listen(...)` SHALL return `error` instead of calling `log.Fatal`. The caller is responsible for handling startup errors. `http.ErrServerClosed` SHALL NOT be treated as an error.

#### Scenario: Listen returns error on bind failure
**WHEN** `Listen` is called on a port that is already in use
**THEN** `Listen` SHALL return a non-nil error describing the bind failure
**AND** the process SHALL NOT be terminated

#### Scenario: Listen returns nil on successful start
**WHEN** `Listen` binds successfully and later the server is shut down via `Shutdown`
**THEN** `Listen` SHALL return `nil`

#### Scenario: Listen signature returns error
**WHEN** any code calls `app.Listen()`
**THEN** the function signature SHALL be `Listen(configs ...ListenConfig) error`

---

### Requirement: LIF-002 — Freeze on first ServeHTTP; panic on registration after freeze

The route table SHALL be frozen atomically on the first call to `App.ServeHTTP`. The router is private and is dispatched only through the app. Any route or middleware registration after freeze SHALL panic with a clear message.

#### Scenario: First ServeHTTP freezes routes
**WHEN** `app.ServeHTTP(w, r)` is called for the first time
**THEN** the route table SHALL be atomically marked as frozen
**AND** subsequent request dispatching SHALL use the frozen table

#### Scenario: Registration after freeze panics
**WHEN** the app has already served a request (table is frozen)
**AND** a handler calls `app.GET("/late", someHandler)`
**THEN** the call SHALL panic with a message containing `"frozen"` or `"cannot register route after ServeHTTP"`

#### Scenario: Registration before freeze succeeds normally
**WHEN** the app has not yet served any request
**AND** a handler calls `app.GET("/ok", handler)`
**THEN** the registration SHALL succeed
**AND** no panic SHALL occur

#### Scenario: Use after freeze panics
**WHEN** the app has already served a request
**AND** `app.Use(middleware)` is called
**THEN** it SHALL panic with a clear message

#### Scenario: Panic after freeze is atomic
**WHEN** two goroutines simultaneously call `ServeHTTP` and `Handle`
**THEN** the freeze SHALL be enforced atomically (one goroutine sees frozen, the other does not)
**AND** no partial or corrupt route table SHALL be observable

---

### Requirement: LIF-003 — Runtime configuration and router state are private

`App` SHALL NOT expose mutable `Config`, `Router`, or `ErrorHandler` fields. `AppConfig` SHALL remain an exported construction input that is copied into private application state, and all runtime routing and middleware state SHALL become immutable at freeze.

#### Scenario: Caller mutation does not change a constructed app
**WHEN** a caller passes an `AppConfig` value to `New` and later mutates its local value
**THEN** the constructed app's effective configuration SHALL remain unchanged

#### Scenario: Mutable runtime fields are not exported
**WHEN** external code imports the package
**THEN** it SHALL NOT be able to assign `app.Config`, `app.Router`, or `app.ErrorHandler`

---

### Requirement: LIF-004 — Duplicate route panics at registration

Registering the exact same method + pattern twice SHALL panic at registration time with a clear message identifying the conflict.

#### Scenario: Exact duplicate method and pattern panics
**WHEN** `app.GET("/users", h1)` is called
**AND** `app.GET("/users", h2)` is called subsequently
**THEN** the second call SHALL panic with a message containing `"duplicate route"` or equivalent

#### Scenario: Canonical trailing-slash duplicate panics
**WHEN** `app.GET("/users", h1)` is registered
**AND** `app.GET("/users/", h2)` is registered subsequently
**THEN** the second registration SHALL panic as a duplicate canonical route

#### Scenario: Same pattern under different methods is allowed
**WHEN** `app.GET("/users", h1)` is called
**AND** `app.POST("/users", h2)` is called
**THEN** both registrations SHALL succeed without panic

---

### Requirement: LIF-005 — Conflicting dynamic route panics

Registering a dynamic route that conflicts with an existing dynamic route (same method, same position, different parameter name) SHALL panic at registration time.

#### Scenario: Conflicting param names panic
**WHEN** `app.GET("/users/:id", h1)` is registered
**AND** `app.GET("/users/:name", h2)` is registered subsequently
**THEN** the second registration SHALL panic with a message about conflicting parameter names

#### Scenario: Same param name is allowed (no conflict)
**WHEN** `app.GET("/users/:id", h1)` is registered
**AND** `app.GET("/users/:id/orders", h2)` is registered
**THEN** both registrations SHALL succeed (different full patterns, same param name in same segment is fine)

---

### Requirement: LIF-006 — Invalid pattern syntax panics

Route patterns with syntactically invalid segments SHALL panic with a descriptive message at registration time.

#### Scenario: Empty param name panics
**WHEN** a pattern like `/users/:/profile` is registered (colon with no name)
**THEN** registration SHALL panic with a message indicating empty parameter name

#### Scenario: Empty wildcard name panics
**WHEN** a pattern like `/assets/*` is registered (star with no name)
**THEN** registration SHALL panic with a message indicating empty wildcard name

#### Scenario: Wildcard followed by more segments panics
**WHEN** a pattern like `/assets/*file/details` is registered (wildcard not final)
**THEN** registration SHALL panic indicating wildcard must be the final segment

#### Scenario: Valid pattern succeeds
**WHEN** `/users/:id` or `/assets/*file` or `/static/path` is registered
**THEN** registration SHALL succeed without panic

---

### Requirement: LIF-007 — Static route trailing-slash aliases

For static patterns ending with or without a trailing slash, the framework SHALL register both canonical forms as aliases pointing to the same route index. These are not considered duplicates.

#### Scenario: Trailing slash alternative is registered automatically
**WHEN** `app.GET("/users", h1)` is registered
**THEN** both `/users` and `/users/` SHALL match the same handler

#### Scenario: Explicit trailing slash pattern also generates alias
**WHEN** `app.GET("/users/", h1)` is registered
**THEN** both `/users/` and `/users` SHALL match the same handler

---

### Requirement: LIF-008 — Nil handler panics at registration

Passing a nil `HandlerFunc` to any route registration method SHALL panic immediately with a clear message.

#### Scenario: Nil handler for GET panics
**WHEN** `app.GET("/test", nil)` is called
**THEN** it SHALL panic with a message containing `"nil handler"`

---

### Requirement: LIF-009 — Syntactically invalid HTTP method panics

Registering a route with a syntactically invalid or empty HTTP method string SHALL panic at registration time.

#### Scenario: Empty method panics
**WHEN** `app.Handle("", "/path", handler)` is called
**THEN** it SHALL panic with a message indicating empty or invalid method

#### Scenario: Method with spaces panics
**WHEN** `app.Handle("GE T", "/path", handler)` is called
**THEN** it SHALL panic with a message indicating invalid method

#### Scenario: Valid extension method is accepted
**WHEN** `app.Handle("CUSTOM", "/path", handler)` is called
**THEN** registration SHALL succeed because the method is a valid RFC 9110 token
