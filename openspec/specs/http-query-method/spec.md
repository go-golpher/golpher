# HTTP QUERY Method Specification

## Purpose

Export the `QUERY` HTTP method constant (`MethodQuery`) following Go's `net/http` naming convention. Provide `App.QUERY` and `Group.QUERY` shorthand registration methods. Ensure the router dispatches `QUERY` as a first-class method through static and dynamic routing, `Allow` header generation, and method-not-allowed responses. Document `QUERY` as safe and idempotent per RFC 10008. Enforce mandatory `Content-Type` for `QUERY` requests. Clarify that media type negotiation, caching, and CORS considerations are the handler's or documentation's responsibility.

## Requirements

### Requirement: QUERY-001 — MethodQuery constant

The package SHALL export a constant `MethodQuery = "QUERY"` following the naming style of Go's `net/http` constants (`http.MethodGet`, `http.MethodPost`, etc.).

#### Scenario: MethodQuery is exported
**WHEN** the package is imported
**THEN** `golpher.MethodQuery` SHALL be defined as the string `"QUERY"`

#### Scenario: MethodQuery is usable with Handle
**WHEN** `app.Handle(golpher.MethodQuery, "/query", handler)` is called
**THEN** the route SHALL be registered for the `QUERY` HTTP method

---

### Requirement: QUERY-002 — App.QUERY and Group.QUERY shorthand

`App` and `Group` SHALL expose a `QUERY(pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc)` shorthand method that delegates to `Handle(MethodQuery, ...)`.

#### Scenario: App.QUERY registers QUERY route
**WHEN** `app.QUERY("/search", handler)` is called
**THEN** it SHALL register a route with method `"QUERY"` and the given pattern

#### Scenario: Group.QUERY registers QUERY route with prefix
**WHEN** `group.QUERY("/search", handler)` is called where `group` has prefix `/api`
**THEN** the full pattern SHALL be `/api/search`
**AND** the method SHALL be `"QUERY"`

#### Scenario: QUERY accepts HandlerFunc and middleware
**WHEN** `app.QUERY("/q", handler, mw1, mw2)` is called
**THEN** the route SHALL have `handler` as the handler and `[mw1, mw2]` as route-level middleware

---

### Requirement: QUERY-003 — Router dispatches QUERY method

The Router SHALL treat `"QUERY"` as a first-class HTTP method in its internal dispatch, static route table, dynamic route trie, and `Allow` header generation.

#### Scenario: QUERY route matches via static index
**WHEN** a `QUERY` route is registered for `/resource`
**AND** a `QUERY` request arrives at `/resource`
**THEN** the request SHALL match and dispatch to the registered handler

#### Scenario: QUERY route matches via dynamic trie
**WHEN** a `QUERY` route is registered for `/resource/:id`
**AND** a `QUERY` request arrives at `/resource/42`
**THEN** the request SHALL match and the `:id` parameter SHALL be `"42"`

#### Scenario: Allow header includes QUERY
**WHEN** a `GET` and a `QUERY` route are registered for `/resource`
**AND** a `POST` request arrives at `/resource`
**THEN** the `Allow` response header SHALL include both `GET` and `QUERY`

#### Scenario: QUERY-only path returns Allow with QUERY on method mismatch
**WHEN** only a `QUERY` route is registered for `/resource`
**AND** a `GET` request arrives at `/resource`
**THEN** the response SHALL have status `405 Method Not Allowed`
**AND** the `Allow` header SHALL include `"QUERY"`

---

### Requirement: QUERY-004 — QUERY is documented as safe and idempotent

The `QUERY` method SHALL be documented (in code comments and user-facing docs) as safe and idempotent per RFC 10008, consistent with `GET`, `HEAD`, `OPTIONS`, and `TRACE`.

#### Scenario: Code comment on MethodQuery states RFC 10008
**WHEN** inspecting the `MethodQuery` constant declaration
**THEN** the comment SHALL reference RFC 10008
**AND** SHALL state that `QUERY` is safe and idempotent

#### Scenario: Documentation states QUERY contract
**WHEN** reading the package documentation
**THEN** it SHALL explain that `QUERY` is semantically equivalent to `GET` in terms of safety and idempotency but carries a request body

---

### Requirement: QUERY-005 — Content-Type is mandatory and content is resource-defined

Per RFC 10008, the framework SHALL reject a `QUERY` request whose `Content-Type` header is absent. Whether empty or malformed content is valid depends on the declared media type and target resource and SHALL be decided by the handler or decoder.

#### Scenario: QUERY request without Content-Type returns 400
**WHEN** a `QUERY` request arrives without a `Content-Type` header
**THEN** the response SHALL have status `400 Bad Request`

#### Scenario: QUERY request with valid body and Content-Type succeeds
**WHEN** a `QUERY` request arrives with `Content-Type: application/query` and a non-empty body
**THEN** the request SHALL be dispatched to the handler normally

#### Scenario: Empty content with declared media type reaches the resource
**WHEN** a `QUERY` request declares a `Content-Type` and has empty content
**THEN** the framework SHALL dispatch it to the handler or decoder
**AND** that resource SHALL decide whether to return `400`, `422`, or a success response

#### Scenario: Body limit applies to QUERY body
**WHEN** the `MaxRequestBodyBytes` limit is set to 1 MiB
**AND** a `QUERY` request arrives with a body larger than 1 MiB
**THEN** the response SHALL be `413 Request Entity Too Large`

---

### Requirement: QUERY-006 — 415 Unsupported Media Type for Accept-Query responsibility

If a resource does not support the query media type indicated by the `Content-Type`, the handler or resource SHALL return `415 Unsupported Media Type`. The framework SHALL document this responsibility but the check is at the handler level, not the framework level.

#### Scenario: Handler returns 415 for unsupported query media type
**WHEN** a `QUERY` handler receives a `Content-Type: application/xml` but only supports `application/sparql-query`
**THEN** the handler SHALL return `415 Unsupported Media Type`
**AND** the framework documentation SHALL indicate that media type negotiation is the handler's responsibility

---

### Requirement: QUERY-007 — URI query parameters remain distinct

The `QUERY` method SHALL NOT conflate the HTTP method name with URI query parameters. URI query parameters accessed via `Request.Query(name)` SHALL remain fully supported and distinct from the `QUERY` method.

#### Scenario: URI query parameters work with QUERY method
**WHEN** a `QUERY` request arrives at `/search?limit=10&offset=0`
**THEN** `request.Query("limit")` SHALL return `"10"`
**AND** `request.Query("offset")` SHALL return `"0"`

#### Scenario: QUERY method and URI query documentation clarifies distinction
**WHEN** reading the package documentation
**THEN** it SHALL explicitly note that `QUERY` (the HTTP method) and URI query parameters are separate concepts

---

### Requirement: QUERY-008 — CORS and caching documentation

The package SHALL document that `QUERY` responses are cacheable only when the cache key incorporates request content and relevant metadata, and that browser CORS requests using `QUERY` require an `OPTIONS` preflight because `QUERY` is not a CORS-safelisted method.

#### Scenario: Documentation covers QUERY caching
**WHEN** reading the package documentation
**THEN** it SHALL state that `QUERY` responses MAY be cached only with a key that includes the request content, `Content-Type`, and relevant selected metadata

#### Scenario: Documentation covers QUERY CORS
**WHEN** reading the package documentation
**THEN** it SHALL state that browser requests using `QUERY` require an `OPTIONS` preflight and must be included in the allowed methods
