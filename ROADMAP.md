# Roadmap

## Core v0.1

- [x] `http.Handler` compatible app
- [x] Express/Fiber-like method helpers
- [x] Middleware chain
- [x] Route groups
- [x] Path params
- [x] `net/http` middleware interop
- [x] `Recover` and `BodyLimit`
- [x] HTTP/2 compatibility through `net/http`

## Next

- Route wildcard support tests and docs
- Response writer commit tracking
- Streaming helpers while preserving `http.Flusher`
- More response helpers: file, download, HTML
- Request binding helpers for JSON, XML, form, headers and params
- First-party logging/request ID middleware
- Benchmarks against Gin, Fiber, Chi and Zinc
- Optional HTTP/3 adapter investigation
