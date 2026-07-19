<a id="readme-top"></a>

<!-- PROJECT SHIELDS -->
[![CI][ci-shield]][ci-url]
[![Coverage][coverage-shield]][coverage-url]
[![codecov][codecov-shield]][codecov-url]
[![Govulncheck][govulncheck-shield]][govulncheck-url]
[![Go Reference][goref-shield]][goref-url]
[![Go Report Card][goreport-shield]][goreport-url]
[![MIT License][license-shield]][license-url]
[![Issues][issues-shield]][issues-url]

> [!WARNING]
> Golpher has not reached a stable release yet. For production applications today, we recommend using one of the mature frameworks that inspire this project, such as [Fiber](https://github.com/gofiber/fiber), [Gin](https://github.com/gin-gonic/gin), or the Go standard library directly with [`net/http`](https://pkg.go.dev/net/http).

<!-- PROJECT LOGO -->
<br />
<div align="center">
  <h3 align="center">Golpher</h3>

  <p align="center">
    A <code>net/http</code>-first Go microframework with Express/Fiber-like DX.
    <br />
    <a href="https://pkg.go.dev/github.com/go-golpher/golpher"><strong>Explore the API docs »</strong></a>
    <br />
    <br />
    <a href="docs/getting-started.md">Getting Started</a>
    &middot;
    <a href="https://github.com/go-golpher/golpher/issues/new?labels=bug">Report Bug</a>
    &middot;
    <a href="https://github.com/go-golpher/golpher/issues/new?labels=enhancement">Request Feature</a>
  </p>
</div>

<!-- TABLE OF CONTENTS -->
<details>
  <summary>Table of Contents</summary>
  <ol>
    <li>
      <a href="#about-the-project">About The Project</a>
      <ul>
        <li><a href="#why-golpher">Why Golpher?</a></li>
        <li><a href="#built-with">Built With</a></li>
        <li><a href="#status">Status</a></li>
      </ul>
    </li>
    <li>
      <a href="#getting-started">Getting Started</a>
      <ul>
        <li><a href="#prerequisites">Prerequisites</a></li>
        <li><a href="#installation">Installation</a></li>
      </ul>
    </li>
    <li><a href="#usage">Usage</a></li>
    <li><a href="#documentation">Documentation</a></li>
    <li><a href="#roadmap">Roadmap</a></li>
    <li><a href="#star-history">Star History</a></li>
    <li><a href="#contributing">Contributing</a></li>
    <li><a href="#license">License</a></li>
    <li><a href="#contact">Contact</a></li>
    <li><a href="#acknowledgments">Acknowledgments</a></li>
  </ol>
</details>

<!-- ABOUT THE PROJECT -->
## About The Project

Golpher is a small Go web framework designed for teams that want modern routing ergonomics without giving up the standard library boundary.

It is inspired by the developer experience of Fiber, Express, Gin, and Zinc, while keeping core Go primitives at the center: `http.Handler`, `http.ResponseWriter`, `*http.Request`, request cancellation, observability middleware, and ordinary `net/http` deployment.

Golpher is built for applications that need framework convenience while remaining interoperable with the broader Go HTTP ecosystem. It supports the HTTP `QUERY` method (RFC 10008) for safe, idempotent queries with a request body.

<p align="right">(<a href="#readme-top">back to top</a>)</p>

### Why Golpher?

- **Standard-library native**: `*golpher.App` implements `http.Handler`.
- **Modern routing DX**: `app.GET`, `app.POST`, `app.QUERY`, route groups, `:params`, `*wildcards`.
- **Middleware chain**: global, group, route, and stdlib `func(http.Handler) http.Handler` middleware.
- **Interop by design**: mount existing `http.Handler` values with `FromHTTPHandler`.
- **HTTP/2 ready**: works through Go's `net/http` TLS/ALPN support.
- **HTTP/3 future-proof**: core stays transport-agnostic so an HTTP/3 adapter can be added later without breaking the API.
- **Security-minded defaults**: server timeouts, `Recover`, `BodyLimit`, 1 MiB default request body limit, route freeze, and validation are present from the start.
- **RFC 10008 QUERY**: first-class `app.QUERY` / `Group.QUERY` with `Content-Type` enforcement.
- **Error masking**: unknown errors produce generic `500` responses by default; the original error reaches only the configured observer.
- **Response tracking**: committed state, status code, bytes written, and opt-in body capture on every write path, including `net/http` adapters.
- **Lazy request body**: `http.MaxBytesReader` wrapping on first read; `Body()` returns `([]byte, error)` and caches the result.

<p align="right">(<a href="#readme-top">back to top</a>)</p>

### Built With

- [Go][go-url]
- [`net/http`][nethttp-url]

<p align="right">(<a href="#readme-top">back to top</a>)</p>

### Status

Golpher is early-stage. The core API is being shaped through spec-driven development and TDD. Expect rapid iteration before a stable `v1`.

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- GETTING STARTED -->
## Getting Started

Follow these steps to install Golpher and run a minimal application.

### Prerequisites

- Go 1.23.6 or newer.

Check your Go version:

```sh
go version
```

### Installation

Install the module in your Go project:

```sh
go get github.com/go-golpher/golpher
```

Then import it:

```go
import "github.com/go-golpher/golpher"
```

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- USAGE EXAMPLES -->
## Usage

### Quick start

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
    return res.SetStatus(http.StatusOK).JSON(map[string]string{
      "id": req.Param("id"),
    })
  })

  // RFC 10008 QUERY method — safe, idempotent, carries a body.
  app.QUERY("/search", func(req *golpher.Request, res *golpher.Response) error {
    var query SearchInput
    if err := req.BodyJSON(&query); err != nil {
      return err
    }
    return res.JSON(searchResults)
  })

  if err := app.Listen(); err != nil {
    log.Fatal(err)
  }
}
```

### Standard `net/http` middleware

```go
app.UseHTTP(func(next http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("X-Powered-By", "golpher")
    next.ServeHTTP(w, r)
  })
})
```

### Mount an existing `http.Handler`

```go
app.Handle(http.MethodGet, "/healthz", golpher.FromHTTPHandler(
  http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    _, _ = w.Write([]byte("ok"))
  }),
))
```

For more examples, see the [Documentation](#documentation).

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- DOCUMENTATION -->
## Documentation

- [Getting started](docs/getting-started.md)
- [Principles and architecture](docs/principles.md)
- [Routing](docs/routing.md)
- [QUERY method (RFC 10008)](docs/query.md)
- [Middleware](docs/middleware.md)
- [Request and response](docs/request-response.md)
- [Error handling](docs/error-handling.md)
- [Interoperability with net/http](docs/interoperability.md)
- [Deployment and protocols](docs/deployment.md)
- [Testing](docs/testing.md)
- [Quality](docs/quality.md)
- [Migration guide](docs/migration.md)

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- BREAKING CHANGES -->
## Breaking changes (v0.0.x)

| Change | Migration |
|---|---|
| `Handler`, `ContextHandlerFunc`, `RawHandlerFunc`, `Ctx`, `Context` removed. Single `HandlerFunc`. | Replace `Handler`/`ContextHandlerFunc` handlers with `func(*Request, *Response) error`. Replace `RawHandlerFunc` with `FromHTTPHandlerFunc`. |
| `App.Get`/`Post`/`Put`/`Patch`/`Delete` (`Handler` variants), `HandleCtx`, `HandleContext`, `Raw` removed. | Use `App.Handle` or the uppercase verb shorthands (`GET`, `POST`, ...) with `HandlerFunc`. |
| Error callbacks receive `(*Request, *Response, error)`. | Update `ErrorHandler` and `ErrorObserver` signatures. |
| `res.Status(code)` renamed to `res.SetStatus(code)`. `res.Status()` now reports the effective status. | Replace all `res.Status(code)` calls with `res.SetStatus(code)`. |
| `Request.Body()` returns `([]byte, error)`. | Replace `req.Body().Bytes()` with `req.Body()`, `req.Body().JSON(v)` with `req.BodyJSON(v)`. |
| `DisableResponseBodyCapture` replaced by `EnableResponseBodyCapture` (default false). | Remove `DisableResponseBodyCapture`; add `EnableResponseBodyCapture: true` if capture is needed. |
| `App.Listen()` returns `error`. | Handle the error: `if err := app.Listen(); err != nil { ... }`. |
| `App.Config`, `App.Router`, `App.ErrorHandler` no longer exported. | Supply configuration through `AppConfig` to `New`. |
| Route/middleware registration after first `ServeHTTP` panics. | Register all routes before serving. |
| Invalid, duplicate, or conflicting route patterns panic at registration. | Fix invalid patterns; see `docs/routing.md`. |
| Unknown errors masked: generic `500` body. | Use `ErrorObserver` to inspect original errors. |
| Default request body limit: 1 MiB. | Set `MaxRequestBodyBytes` to adjust or `-1` to disable. |
| `req.NewError` / `ctx.NewError` removed. | Use `ErrorGolpher{Code: status, Message: msg}`. |
| `BodyLimit` middleware is lazy (no eager read). | Behaviour unchanged for calls through `req.Body()`. |

For detailed migration patterns, see [docs/migration.md](docs/migration.md).

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- ROADMAP -->
## Roadmap

See [ROADMAP.md](ROADMAP.md) for planned work and [open issues][issues-url] for proposed features and known issues.

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- STAR HISTORY -->
## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=go-golpher/golpher&type=Date&theme=dark)](https://star-history.com/#go-golpher/golpher&Date)

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- CONTRIBUTING -->
## Contributing

Contributions are what make the open source community such an amazing place to learn, inspire, and create. Any contributions you make are greatly appreciated.

If you have a suggestion that would make Golpher better, please fork the repository and create a pull request. You can also open an issue with the `enhancement` label.

1. Fork the project.
2. Create your feature branch (`git checkout -b feature/amazing-feature`).
3. Commit your changes using [Conventional Commits](https://www.conventionalcommits.org/) (`git commit -m 'feat: add amazing feature'`).
4. Push to the branch (`git push origin feature/amazing-feature`).
5. Open a pull request.

For project-specific guidance, see [CONTRIBUTING.md](CONTRIBUTING.md).

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- LICENSE -->
## License

Distributed under the MIT License. See [LICENSE](LICENSE) for more information.

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- CONTACT -->
## Contact

Project Link: [https://github.com/go-golpher/golpher][project-url]

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- ACKNOWLEDGMENTS -->
## Acknowledgments

- [Best-README-Template](https://github.com/othneildrew/Best-README-Template) for the README structure.
- [Fiber](https://github.com/gofiber/fiber), [Express](https://expressjs.com/), [Gin](https://github.com/gin-gonic/gin), and [Zinc](https://github.com/zinclabs/zinc) for framework ergonomics inspiration.

<p align="right">(<a href="#readme-top">back to top</a>)</p>

<!-- MARKDOWN LINKS & IMAGES -->
[ci-shield]: https://github.com/go-golpher/golpher/actions/workflows/ci.yml/badge.svg
[ci-url]: https://github.com/go-golpher/golpher/actions/workflows/ci.yml
[coverage-shield]: https://github.com/go-golpher/golpher/actions/workflows/coverage.yml/badge.svg
[coverage-url]: https://github.com/go-golpher/golpher/actions/workflows/coverage.yml
[codecov-shield]: https://codecov.io/gh/go-golpher/golpher/graph/badge.svg
[codecov-url]: https://codecov.io/gh/go-golpher/golpher
[govulncheck-shield]: https://github.com/go-golpher/golpher/actions/workflows/govulncheck.yml/badge.svg
[govulncheck-url]: https://github.com/go-golpher/golpher/actions/workflows/govulncheck.yml
[goref-shield]: https://pkg.go.dev/badge/github.com/go-golpher/golpher.svg
[goref-url]: https://pkg.go.dev/github.com/go-golpher/golpher
[goreport-shield]: https://goreportcard.com/badge/github.com/go-golpher/golpher
[goreport-url]: https://goreportcard.com/report/github.com/go-golpher/golpher
[license-shield]: https://img.shields.io/github/license/go-golpher/golpher.svg?style=flat
[license-url]: https://github.com/go-golpher/golpher/blob/main/LICENSE
[issues-shield]: https://img.shields.io/github/issues/go-golpher/golpher.svg?style=flat
[issues-url]: https://github.com/go-golpher/golpher/issues
[project-url]: https://github.com/go-golpher/golpher
[go-url]: https://go.dev/
[nethttp-url]: https://pkg.go.dev/net/http
