# Interoperability with net/http

Golpher's main constraint is also its main feature: the framework keeps the `net/http` boundary intact.

## App is an http.Handler

```go
var _ http.Handler = golpher.New()
```

This means Golpher works with:

- `http.Server`
- `httptest`
- reverse proxies
- OpenTelemetry and metrics middleware
- standard Go deployment patterns

## Use existing middleware

```go
app.UseHTTP(func(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// before
		next.ServeHTTP(w, r)
		// after
	})
})
```

Errors returned by Golpher handlers are written inside the standard middleware chain, so status-capturing and logging middleware can observe error responses.

## Mount existing handlers

```go
app.Handle(http.MethodGet, "/metrics", golpher.FromHTTPHandler(promHandler))
```

## Access raw objects

```go
rawRequest := req.Raw()
rawWriter := res.Raw()
```

Use raw access when integrating with libraries that require the standard Go types directly.
