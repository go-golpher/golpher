# Error handling

Golpher handlers return `error`.

```go
type HandlerFunc func(*golpher.Request, *golpher.Response) error
```

The router passes returned errors to the configured `ErrorHandler`.

## Returning HTTP errors

Return `ErrorGolpher` from handlers to control the HTTP status code and response body.

```go
app.GET("/private", func(req *golpher.Request, res *golpher.Response) error {
    return golpher.ErrorGolpher{Code: http.StatusUnauthorized, Message: "unauthorized"}
})
```

The `Message` field is sent to the client exactly as-is; the `Code` field maps to the HTTP status line.

## Default error handler

The default error handler writes JSON responses.

For `ErrorGolpher` values:

```json
{
  "code": 401,
  "message": "unauthorized"
}
```

For unknown errors (any `error` that is not `ErrorGolpher`), the default handler returns a generic `500 Internal Server Error` body. The original error message is never sent to the client. Use the `ErrorObserver` to inspect the original error.

## Custom error handler

```go
app := golpher.New(golpher.AppConfig{
    ErrorHandler: func(req *golpher.Request, res *golpher.Response, err error) {
        _ = res.SetStatus(http.StatusInternalServerError).JSON(map[string]string{
            "error": "internal server error",
        })
    },
})
```

## Error observer

An optional `ErrorObserver` receives every framework error with the request and response context before the error handler runs. It fires exactly once per request error. The observer receives the original error (not masked, including `*http.MaxBytesError` for body-limit overflows).

```go
app := golpher.New(golpher.AppConfig{
    ErrorObserver: func(req *golpher.Request, res *golpher.Response, err error) {
        log.Printf("request error: %v", err)
    },
})
```

If the response is already committed, the error handler is skipped but the observer still runs.

## Middleware errors

Middleware can stop the chain by returning an error without calling `next`.

```go
func RequireAuth(next golpher.HandlerFunc) golpher.HandlerFunc {
    return func(req *golpher.Request, res *golpher.Response) error {
        if req.Raw().Header.Get("Authorization") == "" {
            return golpher.ErrorGolpher{Code: http.StatusUnauthorized, Message: "unauthorized"}
        }
        return next(req, res)
    }
}
```

## Panic recovery

Use `Recover()` to convert panics into sanitized `500 Internal Server Error` responses.

```go
app.Use(golpher.Recover())
```

`Recover()` logs the panic value but does not expose it to the client response.
