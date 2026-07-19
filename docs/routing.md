# Routing

Golpher exposes method helpers inspired by Fiber, Gin, Express, and Zinc while staying compatible with `net/http`.

## Method helpers

```go
app.GET("/users", listUsers)
app.POST("/users", createUser)
app.PUT("/users/:id", replaceUser)
app.PATCH("/users/:id", updateUser)
app.DELETE("/users/:id", deleteUser)
app.QUERY("/search", searchQuery) // RFC 10008
```

All shorthands delegate to `Handle`. `Handle` accepts any RFC 9110 valid method string, including extension methods.

```go
app.Handle("CUSTOM", "/path", handler)
```

## Handlers

Routes accept `HandlerFunc`:

```go
type HandlerFunc func(*Request, *Response) error
```

Standard-library handlers mount via `FromHTTPHandler` or `FromHTTPHandlerFunc`:

```go
app.Handle(http.MethodGet, "/metrics", golpher.FromHTTPHandler(promHandler))
```

## Path parameters

Use `:name` in the route pattern and `req.Param(name)` in the handler.

Contract:

- Params are matched by segment.
- Missing params return an empty string.
- Static segments must match exactly.
- Duplicate param names within the same pattern panic at registration.

```go
app.GET("/users/:id", func(req *golpher.Request, res *golpher.Response) error {
    return res.JSON(map[string]string{"id": req.Param("id")})
})
```

## Wildcards

Use `*name` to capture the remainder of the path. Wildcards must be the final segment.

```go
app.GET("/files/*path", func(req *golpher.Request, res *golpher.Response) error {
    return res.String(req.Param("path"))
})
```

Requesting `/files/a/b/c` sets `path` to `a/b/c`.

## Query values

URI query parameters are accessed through the Request wrapper and are distinct from the HTTP method.

```go
app.GET("/search", func(req *golpher.Request, res *golpher.Response) error {
    return res.JSON(map[string]string{"q": req.Query("q")})
})
```

## Groups

Groups attach a prefix and optional middleware to a set of routes.

```go
api := app.Group("/api")
api.GET("/health", func(req *golpher.Request, res *golpher.Response) error {
    return res.String("ok")
})
```

Group middleware runs after global middleware and before route-specific middleware.

Groups support all verb shorthands including `QUERY`:

```go
api := app.Group("/api", authMiddleware)
api.QUERY("/search", searchHandler)
```

## Route validation

All validation runs at registration time, before any request is served.

### Method syntax

Methods are validated as RFC 9110 tokens (case-sensitive, one or more ASCII `tchar` bytes). Empty strings, whitespace, separators, control characters, and non-ASCII bytes panic. Extension methods (`CUSTOM`, `FOOBAR`) are accepted.

### Pattern syntax

- Must start with `/`.
- Segments are split by `/`.
- `:name` declares a named parameter. Name must match `[A-Za-z_][A-Za-z0-9_]*`.
- `*name` declares a terminal wildcard. Name follows the same rules and must appear only in the final position.
- Empty parameter or wildcard names panic (e.g. `:/`, `*`).
- Duplicate parameter names within one pattern panic.

### Nil handler

Registering a nil handler panics with `"golpher: nil handler"`.

### Exact duplicate

Registering the same `method + canonical pattern` twice panics:

```go
app.GET("/users") // OK
app.GET("/users") // panic: duplicate route
```

### Trailing-slash aliasing

Registering `/users` or `/users/` registers both forms mapping to the same route. Explicitly registering the alias panics as a duplicate:

```go
app.GET("/users")  // OK
app.GET("/users/") // panic: duplicate route
```

### Structural conflict

Routes whose segment shape conflicts at the same position panic:

- `:param` vs `*wildcard` at the same position → panic.
- Two `:param` with different names at the same position → panic.
- Same `:param` name (same route) → allowed.
- Static segment and `:param` / `*wildcard` at the same position → allowed (static wins at match time).

```go
app.GET("/users/:id")      // OK
app.GET("/users/:name")    // panic: conflicting param names
```

## Freeze

The first call to `App.ServeHTTP` (or `App.Listen` / `App.Serve`) freezes the route table. Any subsequent route or middleware registration panics:

```go
app.ServeHTTP(w, req)  // freeze
app.GET("/late", h)    // panic: app is frozen
```

Use `app.IsFrozen()` to check the state.

## 404 and 405

Golpher returns:

- `404 Not Found` when no route matches the path.
- `405 Method Not Allowed` when the path exists for another method.

For `405`, Golpher also sets the `Allow` header listing the methods that match the path.
