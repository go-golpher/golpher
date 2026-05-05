# Performance Notes

Golpher preserves the `net/http` compatibility contract while avoiding avoidable per-request allocations in common static-route handlers.

Current hot-path decisions:

- `Request` and `Response` wrappers are reused with `sync.Pool` after each request completes.
- Request state is reset before reuse, including body cache, params, and raw `*http.Request` references; the internal body wrapper remains attached so it can be reused.
- Response state is reset before reuse, including writer, status code, and body buffer.
- Oversized response body buffers are dropped before returning a `Response` to the pool so one large response does not permanently increase pooled memory usage.
- Services that do not inspect `Response.Body()` can set `AppConfig.DisableResponseBodyCapture` to skip response snapshot copies on `Send` and `String`.
- `BodyLimit` fills the request body cache while enforcing the limit, so handlers that call `req.Body()` do not re-read the raw request body.
- Successful handlers avoid `Context` construction; `Context` is created only when error handling needs it.
- Static route matching uses an exact method/path map before the general matcher and preserves the existing trailing-slash-compatible behavior.
- Middleware chains are precompiled after route or app middleware registration; routes with no middleware dispatch directly to the handler.
- `App.Serve(listener)` lets callers provide a pre-created `net.Listener` such as a Unix domain socket without coupling the core router to a specific transport.
- `App.Raw(method, pattern, handler)` registers an opt-in fast route that receives `http.ResponseWriter` and `*http.Request` directly, bypassing Golpher request/response wrappers and native middleware for latency-sensitive endpoints.
- `Response.Bytes` and `Response.JSONBytes` write trusted pre-encoded bytes directly with content metadata and no response body snapshot.

These changes keep the public handler API stable and do not add transport-specific dependencies.
