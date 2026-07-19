# Handler API Unification Specification

## Purpose

Define a single exported handler signature `HandlerFunc func(*Request, *Response) error` as the sole handler type. Consolidate all route registration methods to accept only `HandlerFunc`. Remove legacy `Handler`, `ContextHandlerFunc`, `RawHandlerFunc`, `Ctx`, `Context`, and all `*Ctx`/`*Context`-based overloads. Ensure the `Router` internally dispatches using only `HandlerFunc` and that `Group` methods accept only `HandlerFunc`. Error callbacks receive `*Request` and `*Response` directly.

## Requirements

### Requirement: HAU-001 — Single exported handler signature: HandlerFunc

The package SHALL export exactly one handler type: `HandlerFunc func(*Request, *Response) error`. The following types SHALL be removed from the public API:
- `Handler` (the `*Ctx`-based signature)
- `ContextHandlerFunc`
- `RawHandlerFunc`

`MiddlewareFunc` (`func(HandlerFunc) HandlerFunc`) SHALL remain unchanged.

#### Scenario: HandlerFunc is the only handler type
**WHEN** the package is imported by external code
**THEN** the exported names `Handler`, `ContextHandlerFunc`, and `RawHandlerFunc` SHALL NOT exist
**AND** `HandlerFunc` SHALL be exported with signature `func(*Request, *Response) error`
**AND** `MiddlewareFunc` SHALL remain exported

#### Scenario: Ctx and Context are removed
**WHEN** the package is imported
**THEN** the exported `Ctx` and `Context` types SHALL NOT exist
**AND** request, response, error-handler, and observer APIs SHALL use `*Request` and `*Response` directly

---

### Requirement: HAU-002 — Handler registration uses only HandlerFunc

The following `App` methods SHALL be removed:
- `HandleCtx`
- `HandleContext`
- `Raw`
- `Get` (the `Handler`-accepting variant)
- `Post` (the `Handler`-accepting variant)
- `Put` (the `Handler`-accepting variant)
- `Patch` (the `Handler`-accepting variant)
- `Delete` (the `Handler`-accepting variant)
- `GETContext`
- `POSTContext`
- `PUTContext`
- `PATCHContext`
- `DELETEContext`

The following `App` methods SHALL remain and accept only `HandlerFunc`:
- `Handle` (single registration method)
- `GET`, `POST`, `PUT`, `PATCH`, `DELETE` (shorthand that calls `Handle`)

#### Scenario: Handle accepts HandlerFunc only
**WHEN** `app.Handle(http.MethodGet, "/path", handler)` is called
**THEN** `handler` MUST be of type `HandlerFunc`
**AND** any code attempting to pass a removed type SHALL fail to compile

#### Scenario: Removed methods produce compilation error
**WHEN** existing code calls `app.HandleCtx(...)`, `app.HandleContext(...)`, `app.Raw(...)`, `app.Get(...)`, `app.Post(...)`, `app.Put(...)`, `app.Patch(...)`, `app.Delete(...)` (the `Handler`-accepting forms)
**THEN** compilation SHALL fail with `undefined`

#### Scenario: Verb shorthands remain for HandlerFunc
**WHEN** `app.GET("/path", hf)` is called where `hf` is `HandlerFunc`
**THEN** it SHALL compile and behave identically to `app.Handle(http.MethodGet, "/path", hf)`

---

### Requirement: HAU-003 — Router internal dispatch supports only HandlerFunc

The `Router` SHALL internally store and dispatch routes using only `HandlerFunc`. The `route` struct SHALL consolidate to a single handler field. The route kind constants `routeKindCtx`, `routeKindContext`, and `routeKindRaw` SHALL be removed.

#### Scenario: Route struct has single handler field
**WHEN** a route is registered via any method
**THEN** the route SHALL store exactly one handler of type `HandlerFunc`
**AND** dispatch SHALL call that handler directly without type switching

#### Scenario: Adapters produce HandlerFunc
**WHEN** `FromHTTPHandler(h)` or `FromHTTPHandlerFunc(hf)` is called
**THEN** the return type SHALL be `HandlerFunc`

---

### Requirement: HAU-004 — Group uses only HandlerFunc for registration

`Group` SHALL expose only `HandlerFunc`-based registration. The methods `Handle`, `GET`, `POST`, `PUT`, `PATCH`, and `DELETE` on `Group` SHALL accept only `HandlerFunc`.

#### Scenario: Group.Handle accepts only HandlerFunc
**WHEN** `group.Handle(http.MethodGet, "/path", handler)` is called
**THEN** `handler` MUST be `HandlerFunc`

#### Scenario: Group verb methods accept only HandlerFunc
**WHEN** `group.GET("/path", hf)` is called
**THEN** `hf` MUST be `HandlerFunc`

---

### Requirement: HAU-005 — Router handle methods consolidated

The unexported `Router.handle` SHALL become the single internal registration method. The methods `Router.handleCtx`, `Router.handleContext`, and `Router.handleRaw` SHALL be removed.

#### Scenario: Single internal registration path
**WHEN** a route is registered through any public API
**THEN** internally it SHALL pass through a single registration function that accepts `(method, pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc)`

#### Scenario: Removed internal methods absent
**WHEN** inspecting the `Router` implementation
**THEN** there SHALL be no methods named `handleCtx`, `handleContext`, or `handleRaw`

---

### Requirement: HAU-006 — Error callbacks use Request and Response directly

`ErrorHandlerFunc` SHALL have signature `func(*Request, *Response, error)`, and `ErrorObserverFunc` SHALL have the same arguments. Both SHALL be configured through `AppConfig` and stored privately by `App`.

#### Scenario: ErrorHandlerFunc receives direct request and response
**WHEN** configuring `AppConfig{ErrorHandler: handler}`
**THEN** `handler` SHALL have signature `func(*Request, *Response, error)`

#### Scenario: ErrorObserverFunc receives direct request and response
**WHEN** configuring `AppConfig{ErrorObserver: observer}`
**THEN** `observer` SHALL have signature `func(*Request, *Response, error)`
