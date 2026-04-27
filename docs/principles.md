# Principles and architecture

Golpher is a small HTTP framework built on top of the Go standard library.

## Core contract

`*golpher.App` implements `http.Handler`.

```go
var _ http.Handler = golpher.New()
```

This contract is the main design constraint. Any feature added to Golpher should preserve compatibility with:

- `http.Server`
- `http.Handler`
- `http.ResponseWriter`
- `*http.Request`
- `context.Context` cancellation
- standard Go middleware
- observability libraries that wrap `net/http`

## Design goals

- Provide a concise routing and middleware API.
- Keep the hot path small and predictable.
- Avoid replacing `net/http` with a custom runtime.
- Make standard-library interoperability the default, not an adapter afterthought.
- Keep protocol support aligned with the Go ecosystem.

## Non-goals

- Golpher does not ship a custom HTTP server runtime.
- Golpher does not replace `http.ResponseWriter` or `*http.Request` as the source of truth.
- Golpher does not include database, authentication, template, or full-stack application layers.
- Golpher does not add HTTP/3 to the core package.

## Request lifecycle

1. `App.ServeHTTP` delegates to the router.
2. The router matches `method + path`.
3. Golpher creates request and response wrappers around the original `net/http` objects.
4. Global middleware runs first.
5. Group and route middleware run next.
6. The route handler runs.
7. Returned errors are passed to the configured error handler.

## Compatibility rules

When changing Golpher, preserve these rules:

- Do not discard the original request context.
- Do not write error responses outside the `net/http` middleware chain.
- Do not require users to abandon existing `http.Handler` middleware.
- Do not add transport-specific dependencies to the core package.
- Do not introduce hidden global state for request handling.

## Performance posture

Golpher borrows practical lessons from high-performance Go frameworks while keeping `net/http` compatibility:

- prefer simple data structures until benchmarks prove otherwise;
- avoid reflection in routing and middleware dispatch;
- avoid reading request bodies unless requested or required by middleware;
- keep middleware chaining explicit;
- benchmark changes before adding complexity.
