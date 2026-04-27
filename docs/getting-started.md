# Getting started

Golpher is a small HTTP framework for Go that keeps `net/http` as its foundation.

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

## Start a server

```go
app.Listen()
```

For production, prefer `app.Server(addr)` so you can own lifecycle, TLS, shutdown, and deployment wiring explicitly.

```go
srv := app.Server(":8080")
if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
	log.Fatal(err)
}
```
