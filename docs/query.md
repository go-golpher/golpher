# HTTP QUERY method (RFC 10008)

Golpher supports the HTTP `QUERY` method defined in RFC 10008. `QUERY` is a safe and idempotent method, like `GET`, but carries a request body.

## Method constant

```go
const MethodQuery = "QUERY"
```

`golpher.MethodQuery` is the constant for the QUERY method. Go 1.23 includes no `http.MethodQuery` constant, so Golpher defines its own.

## Registration

Use `App.QUERY` or `Group.QUERY`:

```go
app.QUERY("/search", func(req *golpher.Request, res *golpher.Response) error {
    data, err := req.Body()
    if err != nil {
        return err
    }
    // process query body
    return res.JSON(results)
})

// With group prefix:
api := app.Group("/api")
api.QUERY("/search", searchHandler)
```

## Content-Type requirement

RFC 10008 requires a declared media type for the request body. The framework rejects a `QUERY` request with a missing `Content-Type` header, returning `400 Bad Request` before the handler runs.

```go
// Rejected — no Content-Type header → 400
QUERY /search
Content-Length: 7

q=test

// Accepted — Content-Type declared
QUERY /search
Content-Type: application/query
Content-Length: 7

q=test
```

## Empty body

A `QUERY` request with a declared `Content-Type` and an empty body (or `Content-Length: 0`) is dispatched to the handler. Whether empty content is valid depends on the handler and the declared media type.

```go
// Dispatched to handler (empty body with Content-Type)
QUERY /search
Content-Type: application/query
Content-Length: 0
```

## Body size limit

The configured `MaxRequestBodyBytes` (default 1 MiB) applies to `QUERY` bodies. Exceeding the limit returns `413 Request Entity Too Large`. Use `BodyLimit` middleware for per-route overrides.

## Format negotiation (handler-owned)

The framework does not mandate a query format. Each resource:

- Validates `Content-Type` and returns `415 Unsupported Media Type` for unsupported types.
- Optionally sets an `Accept-Query` structured field in responses (RFC 10008), emitted by the handler.

The framework does not emit `415` or `Accept-Query` automatically.

## Allow header

`QUERY` is included in the `Allow` header when the path exists for `QUERY`. A path registered only for `QUERY` returns `405` with `Allow: QUERY` for a `GET` request.

## URI query parameters

URI query parameters (`?key=value`) are accessed via `Request.Query(name)` and are distinct from the `QUERY` method. `QUERY` carries its semantics in the request body rather than URL parameters.

## CORS preflight

The framework does not implement CORS. Handlers or middleware must handle `OPTIONS` preflight requests. When `QUERY` is used from a browser, the `OPTIONS` preflight handler must include `QUERY` in the `Access-Control-Request-Method` response header.

## Caching

`QUERY` responses are cacheable only if the cache key includes:

- The request body content.
- The `Content-Type` header.
- Any `Vary`-selected metadata.

A `GET`-style cache key using only the URI is incorrect for `QUERY`. Handlers that must prevent caching should set `Cache-Control: no-store`.

## Router dispatch

`QUERY` is a first-class method in the router's static index, dynamic trie, and `Allow` header generation. The method token validator (RFC 9110) accepts `QUERY` without special-casing.
