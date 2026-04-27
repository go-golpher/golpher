# Golpher

[![CI](https://github.com/go-golpher/golpher/actions/workflows/ci.yml/badge.svg)](https://github.com/go-golpher/golpher/actions/workflows/ci.yml)
[![Coverage](https://github.com/go-golpher/golpher/actions/workflows/coverage.yml/badge.svg)](https://github.com/go-golpher/golpher/actions/workflows/coverage.yml)
[![codecov](https://codecov.io/gh/go-golpher/golpher/graph/badge.svg)](https://codecov.io/gh/go-golpher/golpher)
[![Govulncheck](https://github.com/go-golpher/golpher/actions/workflows/govulncheck.yml/badge.svg)](https://github.com/go-golpher/golpher/actions/workflows/govulncheck.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/go-golpher/golpher.svg)](https://pkg.go.dev/github.com/go-golpher/golpher)
[![Go Report Card](https://goreportcard.com/badge/github.com/go-golpher/golpher)](https://goreportcard.com/report/github.com/go-golpher/golpher)

**Golpher is a `net/http`-first Go microframework with Express/Fiber-like DX.**

It is inspired by the ergonomics of Fiber, Express, Gin, Fuego and Zinc, while keeping the standard-library boundary that Go teams rely on: `http.Handler`, `http.ResponseWriter`, `*http.Request`, request cancellation, observability middleware, and ordinary Go deployment.

## Why Golpher?

- **Standard-library native**: `*golpher.App` implements `http.Handler`.
- **Modern routing DX**: `app.GET`, `app.POST`, route groups, and `:params`.
- **Middleware chain**: global, group, route, and stdlib `func(http.Handler) http.Handler` middleware.
- **Interop by design**: mount existing `http.Handler` values with `FromHTTPHandler`.
- **HTTP/2 ready**: works through Go's `net/http` TLS/ALPN support.
- **HTTP/3 future-proof**: core stays transport-agnostic so an HTTP/3 adapter can be added later without breaking the API.
- **Security-minded defaults**: server timeouts, `Recover`, and `BodyLimit` are available from the start.

## Installation

```bash
go get github.com/go-golpher/golpher
```

## Quick start

```go
package main

import (
	"net/http"

	"github.com/go-golpher/golpher"
)

func main() {
	app := golpher.New()

	app.Use(golpher.Recover())
	app.Use(golpher.BodyLimit(2 << 20)) // 2 MB

	app.GET("/", func(req *golpher.Request, res *golpher.Response) error {
		return res.JSON(map[string]string{"message": "hello, golpher"})
	})

	app.GET("/users/:id", func(req *golpher.Request, res *golpher.Response) error {
		return res.Status(http.StatusOK).JSON(map[string]string{
			"id": req.Param("id"),
		})
	})

	app.Listen()
}
```

## Standard `net/http` middleware

```go
app.UseHTTP(func(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Powered-By", "golpher")
		next.ServeHTTP(w, r)
	})
})
```

## Mount an existing `http.Handler`

```go
app.Handle(http.MethodGet, "/healthz", golpher.FromHTTPHandler(
	http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}),
))
```

## Documentation

- [Getting started](docs/getting-started.md)
- [Routing](docs/routing.md)
- [Middleware](docs/middleware.md)
- [Interoperability with net/http](docs/interoperability.md)
- [Deployment and protocols](docs/deployment.md)
- [Quality](docs/quality.md)

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=go-golpher/golpher&type=Date&theme=dark)](https://star-history.com/#go-golpher/golpher&Date)

## Status

Golpher is early-stage. The core API is being shaped through spec-driven development and TDD. Expect rapid iteration before a stable `v1`.

## License

MIT. See [LICENSE](LICENSE).
