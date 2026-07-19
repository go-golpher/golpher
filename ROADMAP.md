# Roadmap

## Core v0.1

- [x] `http.Handler` compatible app
- [x] Express/Fiber-like method helpers
- [x] Middleware chain
- [x] Route groups
- [x] Path params and wildcards
- [x] `net/http` middleware interop
- [x] `Recover` and `BodyLimit`
- [x] HTTP/2 compatibility through `net/http`
- [x] HTTP `QUERY` method (RFC 10008)
- [x] Response tracking (committed, status, bytes written)
- [x] Response body capture opt-in
- [x] Safe header assignment (`http.Header.Set`)
- [x] Lazy `http.MaxBytesReader` body enforcement
- [x] Route validation (syntax, duplicate, structural conflict, nil handler)
- [x] App freeze on first `ServeHTTP`
- [x] `Listen` returns `error`
- [x] Single `HandlerFunc` API
- [x] Error masking and `ErrorObserver`
- [x] Unified error callbacks (`*Request`, `*Response`, `error`)

## Next

- Path-scoped middleware
- Nested route groups
- Matched route pattern metadata for observability
- First-party logging/request ID/CORS middleware
- Streaming helpers while preserving `http.Flusher`
- More response helpers: file, download, HTML
- Request binding helpers for form, headers, and params
- Benchmarks against Gin, Fiber, Chi, and Zinc
- Optional HTTP/3 adapter investigation
