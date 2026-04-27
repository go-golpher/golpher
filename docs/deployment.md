# Deployment and protocols

Golpher runs on top of `net/http`, so deployment stays ordinary Go.

## Server defaults

`app.Server(addr)` creates an `http.Server` with practical default timeouts:

- `ReadHeaderTimeout`: 5s
- `ReadTimeout`: 10s
- `WriteTimeout`: 30s
- `IdleTimeout`: 120s

```go
srv := app.Server(":8080")
```

You can pass `AppConfig` to tune defaults.

```go
app := golpher.New(golpher.AppConfig{
	ReadHeaderTimeout: 3 * time.Second,
	ReadTimeout:       10 * time.Second,
	WriteTimeout:      30 * time.Second,
	IdleTimeout:       120 * time.Second,
})
```

## Graceful shutdown

Golpher exposes a small wrapper over `http.Server.Shutdown`.

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

if err := app.Shutdown(ctx, srv); err != nil {
	log.Fatal(err)
}
```

## HTTP/2

HTTP/2 is handled by Go's standard `net/http` server when TLS/ALPN is configured. Golpher does not need a custom transport for this.

## HTTP/3

HTTP/3 is intentionally not part of the core package. The core remains `http.Handler`-first so an optional QUIC/HTTP3 adapter can be introduced later without forcing every user to depend on it.

## Performance posture

Golpher borrows the practical lessons of high-performance Go frameworks without abandoning compatibility:

- keep the hot path small;
- avoid reflection in routing;
- build middleware chains predictably;
- read request bodies only when requested;
- benchmark before adding complexity.
