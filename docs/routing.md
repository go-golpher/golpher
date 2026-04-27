# Routing

Golpher exposes short route helpers inspired by Fiber, Gin, Express and Zinc while staying compatible with `net/http`.

## Method helpers

```go
app.GET("/users", listUsers)
app.POST("/users", createUser)
app.PUT("/users/:id", replaceUser)
app.PATCH("/users/:id", updateUser)
app.DELETE("/users/:id", deleteUser)
```

## Path parameters

Use `:name` in the route pattern and `req.Param(name)` in the handler.

Contract:

- Params are matched by segment.
- Missing params return an empty string.
- Static segments must match exactly.

```go
app.GET("/users/:id", func(req *golpher.Request, res *golpher.Response) error {
	return res.JSON(map[string]string{"id": req.Param("id")})
})
```

## Query values

```go
app.GET("/search", func(req *golpher.Request, res *golpher.Response) error {
	return res.JSON(map[string]string{"q": req.Query("q")})
})
```

## Groups

Groups attach a prefix and optional middleware to a set of routes.

```go
api := app.Group("/api")
api.GET("/health", func(req *golpher.Request, res *golpher.Response) error {
	return res.String("ok")
})
```

Group middleware runs after global middleware and before route-specific middleware.

## 404 and 405

Golpher returns:

- `404 Not Found` when no route matches the path.
- `405 Method Not Allowed` when the path exists for another method.

For `405`, Golpher also sets the `Allow` header.
