# Performance Notes

Golpher preserves the `net/http` compatibility contract while avoiding avoidable per-request allocations in common static-route handlers.

Current hot-path decisions:

- `Request` and `Response` wrappers are reused with `sync.Pool` after each request completes.
- Request state is reset before reuse, including body cache, params, and raw `*http.Request` references.
- Response state is reset before reuse, including writer, status code, and body buffer.
- Successful handlers avoid `Context` construction; `Context` is created only when error handling needs it.
- Static route matching uses a fast path and preserves the existing trailing-slash-compatible behavior.
- Middleware chains are built lazily; routes with no app or route middleware dispatch directly to the handler.

These changes keep the public handler API stable and do not add transport-specific dependencies.
