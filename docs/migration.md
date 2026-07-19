# Migration guide

This document covers mechanical migration patterns from earlier Golpher APIs to the current surface. Each section lists a removed or changed API and the replacement.

## Handler API collapse

**Removed types:** `Handler`, `ContextHandlerFunc`, `RawHandlerFunc`, `Ctx`, `Context`.

**Removed methods:** `HandleCtx`, `HandleContext`, `Raw`, `Get`, `Post`, `Put`, `Patch`, `Delete` (the `Handler`-accepting variants), `GETContext`, `POSTContext`, `PUTContext`, `PATCHContext`, `DELETEContext`.

**All handlers now use `HandlerFunc`:**

```go
// Before
app.Get("/path", func(c *golpher.Ctx) error { ... })

// After
app.GET("/path", func(req *golpher.Request, res *golpher.Response) error {
    return nil
})
```

**RawHandlerFunc → FromHTTPHandlerFunc:**

```go
// Before
app.Raw(http.MethodPost, "/events", func(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusAccepted)
})

// After
app.Handle(http.MethodPost, "/events", golpher.FromHTTPHandlerFunc(
    func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusAccepted)
    },
))
```

**Error callbacks receive `*Request` and `*Response` directly:**

```go
// Before
ErrorHandler: func(ctx *golpher.Context, err error) { ... }

// After
ErrorHandler: func(req *golpher.Request, res *golpher.Response, err error) { ... }
```

## `Status` → `SetStatus`

The chaining status setter is renamed to `SetStatus`. `Status()` now reports the effective status instead of setting it.

```go
// Before
return res.Status(http.StatusCreated).JSON(payload)

// After
return res.SetStatus(http.StatusCreated).JSON(payload)

// Inspecting status (zero-argument):
res.Status() // returns 0 (uncommitted), 200 (committed implicit), or the staged code
```

## `Request.Body()` signature

`Body()` now returns `([]byte, error)` instead of `*Body`. Decoding helpers move to `Request`.

```go
// Before
data := req.Body().Bytes()
req.Body().JSON(&v)
req.Body().XML(&v)

// After
data, err := req.Body()
err := req.BodyJSON(&v)
err := req.BodyXML(&v)
```

## `DisableResponseBodyCapture` → `EnableResponseBodyCapture`

Body capture is opt-in and disabled by default.

```go
// Before — capture on by default, opt-out:
AppConfig{DisableResponseBodyCapture: true}

// After — capture off by default, opt-in:
AppConfig{EnableResponseBodyCapture: true}

// Removal of opt-out with capture-on default:
// If you previously set DisableResponseBodyCapture: false (capture on),
// replace with EnableResponseBodyCapture: true.
// If you previously relied on capture being on by default,
// add EnableResponseBodyCapture: true explicitly.
```

## `Listen` returns `error`

```go
// Before
app.Listen()

// After
if err := app.Listen(); err != nil {
    log.Fatal(err)
}
```

## `req.NewError` removed

`req.NewError` and `ctx.NewError` are removed. Use `ErrorGolpher` directly:

```go
// Before
return req.NewError(http.StatusUnauthorized, "unauthorized")

// After
return golpher.ErrorGolpher{Code: http.StatusUnauthorized, Message: "unauthorized"}
```

## Private app state

`App.Config`, `App.Router`, and `App.ErrorHandler` fields are no longer exported. Configuration is supplied through `AppConfig` passed to `New` and is immutable after construction.

```go
// Before
app := golpher.New()
app.Config.Port = 9090 // mutable field access

// After
app := golpher.New(golpher.AppConfig{Port: 9090})
// config is private; use AppConfig exclusively
```

## Error masking

The default error handler no longer leaks unknown error messages to the client. Non-`ErrorGolpher` errors produce a generic `500 Internal Server Error` body. The original error is visible only through the `ErrorObserver`.

```go
// Before — unknown errors leaked err.Error() to the client
// After — unknown errors are masked; generic 500 response
```

## `MaxRequestBodyBytes` default

The default request body limit is 1 MiB. Previously there was no framework-enforced limit.

```go
// 0 (zero) → 1 MiB default
// negative (e.g. -1) → unlimited
// positive → enforce that many bytes

AppConfig{MaxRequestBodyBytes: 10 << 20}  // 10 MiB
AppConfig{MaxRequestBodyBytes: -1}         // unlimited
```

## Freeze panic

Route and middleware registration after the first `ServeHTTP` call panics. Register all routes before serving.

```go
app := golpher.New()
app.GET("/users", listUsers)   // OK — before freeze
app.ServeHTTP(w, req)          // freeze
app.GET("/late", handler)      // panic: app is frozen
```

## Route validation panics

Invalid, duplicate, or conflicting route patterns now panic at registration time. Previously these were silently accepted or caused undefined behaviour.

See `docs/routing.md` for the complete validation rules.

## `BodyLimit` middleware (lazy)

`BodyLimit` no longer eagerly reads the body. It sets a per-route limit that applies on first read via `http.MaxBytesReader`. Semantics are the same for well-behaved handlers; handlers that read `req.Body()` directly (not through `BodyJSON`/`BodyXML`) must handle `*http.MaxBytesError` via `errors.As`.

```go
// Behaviour unchanged for handlers that call req.Body():
app.Use(golpher.BodyLimit(64 * 1024))
```

## QUERY method

Add `QUERY` routes where a safe, idempotent method with a request body is needed:

```go
app.QUERY("/search", searchHandler)
```

See `docs/query.md` for the `Content-Type` requirement and caching notes.
