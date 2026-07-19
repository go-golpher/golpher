# Request and response

Handlers receive thin wrappers around standard Go HTTP objects.

```go
func(req *golpher.Request, res *golpher.Response) error
```

## Request contract

`Request` wraps the original `*http.Request`.

### Raw request

Use `Raw()` when a library requires `*http.Request`.

```go
raw := req.Raw()
```

### Context

`Context()` returns the native request context. Client cancellation, deadlines, and request-scoped values come from the original request.

```go
select {
case <-req.Context().Done():
    return req.Context().Err()
default:
}
```

### Headers

```go
authorization := req.Raw().Header.Get("Authorization")
allHeaders := req.Headers()
```

### Route params

```go
app.GET("/users/:id", func(req *golpher.Request, res *golpher.Response) error {
    return res.String(req.Param("id"))
})
```

Missing params return an empty string.

### Query values

```go
page := req.Query("page")
```

### Body

`Body()` reads and caches the request body. Repeated calls return the same cached bytes and error. The body is bounded by the configured `MaxRequestBodyBytes` (default 1 MiB) via a lazy `http.MaxBytesReader`.

```go
data, err := req.Body()
```

Decode JSON:

```go
var input CreateUserInput
if err := req.BodyJSON(&input); err != nil {
    return golpher.ErrorGolpher{Code: http.StatusBadRequest, Message: "invalid JSON body"}
}
```

Decode XML:

```go
var input CreateUserInput
if err := req.BodyXML(&input); err != nil {
    return golpher.ErrorGolpher{Code: http.StatusBadRequest, Message: "invalid XML body"}
}
```

When the body exceeds the configured limit, `Body()` returns a `*http.MaxBytesError`. The framework renders `413 Request Entity Too Large` for such errors when the response is not committed.

## Response contract

`Response` wraps the original `http.ResponseWriter` with tracking for committed state, status code, and bytes written.

### Raw writer

```go
writer := res.Raw()
```

Use this for integrations that need the standard writer directly. Writes through `Raw()` are tracked (committed, status, bytes written, capture). Use `http.ResponseController(res.Raw())` to access optional capabilities such as `Flush` or `Hijack`.

### Headers

```go
res.Header().Set("X-Service", "golpher")
```

### Status

`SetStatus(code)` stages the status code for the next write. After the response is committed, `SetStatus` has no effect on the wire.

```go
return res.SetStatus(http.StatusCreated).JSON(payload)
```

`Status()` returns the effective status (staged code, or `200` if committed without an explicit status, or `0` if uncommitted).

```go
code := res.Status()
```

### Committed

`Committed()` reports whether the response header has been written to the wire.

```go
if res.Committed() {
    // header already sent
}
```

### Bytes written

`BytesWritten()` returns the total number of bytes successfully written to the wire.

```go
n := res.BytesWritten()
```

### Send bytes

```go
return res.Send([]byte("ok"))
```

### Bytes

Write bytes with status, content type, and content length.

```go
return res.Bytes(http.StatusOK, "application/octet-stream", payload)
```

### String

```go
return res.String("hello")
```

### JSON

```go
return res.JSON(map[string]string{"status": "ok"})
```

Write pre-encoded JSON bytes:

```go
return res.JSONBytes([]byte(`{"status":"ok"}`))
```

`JSONBytes` is useful for hot paths that already have serialized JSON and want to avoid `encoding/json` work. It does not validate or escape the input; only use it with trusted, pre-serialized JSON bytes.

### XML

```go
return res.XML(payload)
```

### Redirect

```go
return res.Redirect("/login", http.StatusTemporaryRedirect)
```

### Body snapshot

`Body()` and `BodyString()` expose the captured response body when capture is enabled.

```go
body := res.Body()
text := res.BodyString()
```

Body capture is opt-in via `AppConfig`:

```go
app := golpher.New(golpher.AppConfig{
    EnableResponseBodyCapture: true,
})
```

When capture is disabled (default), `Body()` returns `nil` and `BodyString()` returns `""` regardless of what was written. When capture is enabled, it covers `Send`, `String`, `JSON`, `JSONBytes`, `Bytes`, `XML`, `Redirect`, and writes through `FromHTTPHandler`/`FromHTTPHandlerFunc` adapters.
