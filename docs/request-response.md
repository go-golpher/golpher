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

`Body()` reads and caches the request body. Repeated calls return the same cached body.

```go
data := req.Body().Bytes()
```

Decode JSON:

```go
var input CreateUserInput
if err := req.Body().JSON(&input); err != nil {
	return req.NewError(http.StatusBadRequest, "invalid JSON body")
}
```

Decode XML:

```go
var input CreateUserInput
if err := req.Body().XML(&input); err != nil {
	return req.NewError(http.StatusBadRequest, "invalid XML body")
}
```

## Response contract

`Response` wraps the original `http.ResponseWriter`.

### Raw writer

```go
writer := res.Raw()
```

Use this for integrations that need the standard writer directly.

### Headers

```go
res.Header().Set("X-Service", "golpher")
```

### Status

`Status(code)` stores the status code for the next write.

```go
return res.Status(http.StatusCreated).JSON(payload)
```

### Send bytes

```go
return res.Send([]byte("ok"))
```

### String

```go
return res.String("hello")
```

### JSON

```go
return res.JSON(map[string]string{"status": "ok"})
```

### XML

```go
return res.XML(payload)
```

### Redirect

```go
return res.Redirect("/login", http.StatusTemporaryRedirect)
```

### Body snapshot

`Body()` and `BodyString()` expose the response body written through `Send` and helpers that use `Send`.

```go
body := res.Body()
text := res.BodyString()
```

This is useful for middleware and tests. Direct writes through `res.Raw()` are not captured by the snapshot.
