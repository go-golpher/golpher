package golpher

import (
	"net/http"
	"strings"
)

// Group represents a route group with a shared prefix and middleware.
type Group struct {
	app         *App
	prefix      string
	middlewares []MiddlewareFunc
}

// Group creates a new route group with the given prefix and optional
// middleware. Routes registered on the group are prefixed automatically.
func (app *App) Group(prefix string, middlewares ...MiddlewareFunc) *Group {
	return &Group{
		app:         app,
		prefix:      normalizePrefix(prefix),
		middlewares: append([]MiddlewareFunc(nil), middlewares...),
	}
}

// Use appends middleware to the group.
func (group *Group) Use(middlewares ...MiddlewareFunc) {
	group.middlewares = append(group.middlewares, middlewares...)
}

// Handle registers a route with the given method and pattern, prefixed
// by the group's prefix, and with the group's middleware prepended.
func (group *Group) Handle(method, pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	chain := append(append([]MiddlewareFunc(nil), group.middlewares...), middlewares...)
	group.app.Handle(method, joinPaths(group.prefix, pattern), handler, chain...)
}

// GET registers a GET route on the group.
func (group *Group) GET(pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	group.Handle(http.MethodGet, pattern, handler, middlewares...)
}

// POST registers a POST route on the group.
func (group *Group) POST(pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	group.Handle(http.MethodPost, pattern, handler, middlewares...)
}

// PUT registers a PUT route on the group.
func (group *Group) PUT(pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	group.Handle(http.MethodPut, pattern, handler, middlewares...)
}

// PATCH registers a PATCH route on the group.
func (group *Group) PATCH(pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	group.Handle(http.MethodPatch, pattern, handler, middlewares...)
}

// DELETE registers a DELETE route on the group.
func (group *Group) DELETE(pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	group.Handle(http.MethodDelete, pattern, handler, middlewares...)
}

// QUERY registers a QUERY route on the group (RFC 10008).
func (group *Group) QUERY(pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	group.Handle(MethodQuery, pattern, handler, middlewares...)
}

func normalizePrefix(prefix string) string {
	if prefix == "" || prefix == "/" {
		return ""
	}
	return "/" + strings.Trim(prefix, "/")
}

func joinPaths(prefix, pattern string) string {
	if pattern == "" || pattern == "/" {
		if prefix == "" {
			return "/"
		}
		return prefix
	}
	return normalizePrefix(prefix) + "/" + strings.Trim(pattern, "/")
}
