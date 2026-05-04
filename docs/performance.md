# Performance Notes

Golpher preserves the `net/http` compatibility contract while avoiding avoidable per-request allocations in common static-route handlers.

Current hot-path decisions:

- `Request` and `Response` wrappers are reused with `sync.Pool` after each request completes.
- Request state is reset before reuse, including body cache, params, and raw `*http.Request` references; the internal body wrapper remains attached so it can be reused.
- Response state is reset before reuse, including writer, status code, and body buffer.
- Oversized response body buffers are dropped before returning a `Response` to the pool so one large response does not permanently increase pooled memory usage.
- Successful handlers avoid `Context` construction; `Context` is created only when error handling needs it.
- Static route matching uses a fast path and preserves the existing trailing-slash-compatible behavior.
- Middleware chains are precompiled after route or app middleware registration; routes with no middleware dispatch directly to the handler.

These changes keep the public handler API stable and do not add transport-specific dependencies.
