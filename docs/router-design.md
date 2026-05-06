# Router Design

Golpher optimizes for a Fiber-like developer experience while preserving `net/http` compatibility.

## Matching model

Routes are matched by specificity, not registration order:

1. static exact routes
2. dynamic parameter routes (`:id`)
3. wildcard catch-all routes (`*path`)

This keeps the hot path simple and predictable. A static route such as `/health` wins over `/:id` even if `/:id` was registered first.

## Internal indexes

The router keeps separate indexes:

- static routes: `method -> path -> handler`
- dynamic routes: `method -> segment tree`

Static routes do not store dynamic metadata. Dynamic routes store compiled segment metadata and param names only.

## Handler paths

The runtime keeps distinct dispatch paths:

- raw handler path: `func(http.ResponseWriter, *http.Request)`
- native Golpher path: `func(*Request, *Response) error`
- stdlib middleware bridge path: `UseHTTP` wraps native handlers only when registered
- context path: `func(*Ctx) error` or `func(*Ctx, *Request, *Response) error`

The fast path does not pay for `UseHTTP` when no stdlib middleware is registered.

## Public handler options

Golpher supports three styles:

```go
func(req *Request, res *Response) error
func(ctx *Ctx, req *Request, res *Response) error
type Handler func(*Ctx) error
```

`Request` and `Response` remain separate objects; `Ctx` is an optional request-scoped convenience object, not a replacement storage owner.
