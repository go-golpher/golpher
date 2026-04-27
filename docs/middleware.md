# Middleware

Golpher middleware uses a small chain model:

```go
type MiddlewareFunc func(golpher.HandlerFunc) golpher.HandlerFunc
```

## Global middleware

```go
app.Use(func(next golpher.HandlerFunc) golpher.HandlerFunc {
	return func(req *golpher.Request, res *golpher.Response) error {
		res.Header().Set("X-App", "golpher")
		return next(req, res)
	}
})
```

Execution order is the same as registration order. Code before `next` runs from first to last; code after `next` unwinds from last to first.

## Group middleware

```go
api := app.Group("/api", authMiddleware)
api.GET("/me", currentUser)
```

## Short-circuiting

A middleware can stop the chain by returning an error before calling `next`.

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

## Built-in middleware

### Recover

`Recover()` converts panics into sanitized `500 Internal Server Error` responses.

```go
app.Use(golpher.Recover())
```

### BodyLimit

`BodyLimit(maxBytes)` rejects payloads larger than the configured size with `413 Payload Too Large`.

```go
app.Use(golpher.BodyLimit(2 << 20)) // 2 MB
```

## Standard-library middleware

Use `UseHTTP` for existing `net/http` middleware.

```go
app.UseHTTP(existingMiddleware)
```

Errors returned by Golpher handlers are written inside the standard middleware chain, so status-capturing middleware can observe error responses.
