# Testing

Golpher applications can be tested with the standard `net/http/httptest` package.

## Test a route

```go
func TestHello(t *testing.T) {
	app := golpher.New()
	app.GET("/hello", func(req *golpher.Request, res *golpher.Response) error {
		return res.String("hello")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
```

## Test middleware

```go
app.Use(func(next golpher.HandlerFunc) golpher.HandlerFunc {
	return func(req *golpher.Request, res *golpher.Response) error {
		res.Header().Set("X-Test", "ok")
		return next(req, res)
	}
})
```

Assert the observable HTTP response, not private internals.

## Test request cancellation

```go
ctx, cancel := context.WithCancel(context.Background())
req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
cancel()
```

Golpher exposes the original request context through `req.Context()`.

## Test HTTP/2

Use `httptest.NewUnstartedServer` with `EnableHTTP2 = true`.

```go
server := httptest.NewUnstartedServer(app)
server.EnableHTTP2 = true
server.StartTLS()
defer server.Close()
```

## Coverage expectations

Coverage should include behavior contracts, not only happy paths.

Prioritize tests for:

- method helpers;
- route groups;
- params;
- middleware ordering and short-circuiting;
- `net/http` middleware interoperability;
- request and response helpers;
- body limits;
- recovery behavior;
- server configuration.

Run coverage locally:

```bash
go test -covermode=atomic -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```
