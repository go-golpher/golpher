# Getting started

Golpher is a small HTTP framework for Go that keeps `net/http` as its foundation.

## Contract

- `*golpher.App` implements `http.Handler`.
- Handlers receive `*golpher.Request` and `*golpher.Response` wrappers.
- Handlers return `error`; returned errors are handled centrally.
- The original `*http.Request.Context()` is preserved.

## Install

```bash
go get github.com/go-golpher/golpher
```

## Create an app

```go
app := golpher.New()
```

`App` implements `http.Handler`, so it can be used with `httptest`, `http.Server`, reverse-proxy setups, observability middleware, and existing Go tooling.

## Register a route

```go
app.GET("/hello", func(req *golpher.Request, res *golpher.Response) error {
    return res.String("hello")
})
```

Handlers return `error`. This lets Golpher centralize error handling while keeping handler code short.

## Add middleware

```go
app.Use(golpher.Recover())
app.Use(golpher.BodyLimit(2 << 20))
```

Use `UseHTTP` for existing standard-library middleware.

```go
app.UseHTTP(func(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("X-Service", "golpher")
        next.ServeHTTP(w, r)
    })
})
```

## Start a server

```go
if err := app.Listen(); err != nil {
    log.Fatal(err)
}
```

For production, prefer `app.Server(addr)` so you can own lifecycle, TLS, shutdown, and deployment wiring explicitly.

```go
srv := app.Server(":8080")
if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
    log.Fatal(err)
}
```

## QUERY method

Register a handler for the HTTP `QUERY` method (RFC 10008):

```go
app.QUERY("/search", func(req *golpher.Request, res *golpher.Response) error {
    var query SearchInput
    if err := req.BodyJSON(&query); err != nil {
        return err
    }
    return res.JSON(results)
})
```

`QUERY` requests require a `Content-Type` header; the framework rejects missing headers with `400 Bad Request`.
