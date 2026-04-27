# Error handling

Golpher handlers return `error`.

```go
type HandlerFunc func(*golpher.Request, *golpher.Response) error
```

The router passes returned errors to the configured `ErrorHandler`.

## Returning HTTP errors

Use `req.NewError` inside handlers:

```go
app.GET("/private", func(req *golpher.Request, res *golpher.Response) error {
	return req.NewError(http.StatusUnauthorized, "unauthorized")
})
```

You can also use the context helper where a `*golpher.Context` is available:

```go
return ctx.NewError(http.StatusConflict, "conflict")
```

## Default error handler

The default error handler writes JSON responses.

For `ErrorGolpher` values:

```json
{
  "code": 401,
  "message": "unauthorized"
}
```

For unknown errors, the current default handler returns `500 Internal Server Error` with the error message. This is convenient during early development, but production applications should configure a custom error handler that masks internal details.

## Custom error handler

```go
app := golpher.New(golpher.AppConfig{
	ErrorHandler: func(ctx *golpher.Context, err error) {
		_ = ctx.Response.Status(http.StatusInternalServerError).JSON(map[string]string{
			"error": "internal server error",
		})
	},
})
```

## Middleware errors

Middleware can stop the chain by returning an error without calling `next`.

```go
func RequireAuth(next golpher.HandlerFunc) golpher.HandlerFunc {
	return func(req *golpher.Request, res *golpher.Response) error {
		if req.Raw().Header.Get("Authorization") == "" {
			return req.NewError(http.StatusUnauthorized, "unauthorized")
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
