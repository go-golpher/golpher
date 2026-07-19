# Performance Notes

Golpher preserves the `net/http` compatibility contract while avoiding avoidable per-request allocations in common static-route handlers.

Current hot-path decisions:

- `Request` and `Response` wrappers are reused with `sync.Pool` after each request completes.
- Request state is reset before reuse, including body cache, params, and raw `*http.Request` references.
- Response state is reset before reuse, including writer, status code, and body buffer.
- Oversized response body buffers are dropped before returning a `Response` to the pool so one large response does not permanently increase pooled memory usage.
- Body capture is disabled by default, so handlers that do not inspect `Response.Body()` incur no capture overhead. Enable with `AppConfig.EnableResponseBodyCapture: true` when needed.
- `BodyLimit` sets a per-route limit that applies on first read; no eager read or wrap occurs.
- Successful handlers use a single `HandlerFunc` path; no intermediate wrapper allocation.
- Static route matching uses an exact method/path map, preserves trailing-slash-compatible behavior, and intentionally wins over dynamic params for predictable specificity-first dispatch.
- Middleware chains are precompiled after route or app middleware registration; routes with no middleware dispatch directly to the handler.
- `App.Serve(listener)` lets callers provide a pre-created `net.Listener` such as a Unix domain socket without coupling the core router to a specific transport.
- `Response.Bytes` and `Response.JSONBytes` write trusted pre-encoded bytes directly with content metadata and no response body snapshot.
- Dynamic route matching uses route-time compiled segments, scans the request path once without `strings.Split`, and stores param values in the pooled `Request`; `Param` resolves against route-owned param names, avoiding per-match param-map allocation in the normal handler path.
- `Response.String` writes strings without converting them through an allocating `[]byte` copy and uses a prebuilt text/plain header value for the common plain-text hot path.

These choices keep the public handler API stable and do not add transport-specific dependencies.
