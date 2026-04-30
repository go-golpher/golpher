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

- Path-scoped middleware
- Nested route groups
- Matched route pattern metadata for observability
- Route wildcard support tests and docs
- First-party logging/request ID/CORS middleware
- Response writer commit tracking
- Streaming helpers while preserving `http.Flusher`
- More response helpers: file, download, HTML
- Request binding helpers for JSON, XML, form, headers and params
- Benchmarks against Gin, Fiber, Chi and Zinc
- Optional HTTP/3 adapter investigation
