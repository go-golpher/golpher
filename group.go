package golpher

import (
	"net/http"
	"strings"
)

type Group struct {
	app         *App
	prefix      string
	middlewares []MiddlewareFunc
}

func (app *App) Group(prefix string, middlewares ...MiddlewareFunc) *Group {
	return &Group{
		app:         app,
		prefix:      normalizePrefix(prefix),
		middlewares: append([]MiddlewareFunc(nil), middlewares...),
	}
}

func (group *Group) Use(middlewares ...MiddlewareFunc) {
	group.middlewares = append(group.middlewares, middlewares...)
}

func (group *Group) Handle(method, pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	chain := append(append([]MiddlewareFunc(nil), group.middlewares...), middlewares...)
	group.app.Handle(method, joinPaths(group.prefix, pattern), handler, chain...)
}

func (group *Group) GET(pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	group.Handle(http.MethodGet, pattern, handler, middlewares...)
}

func (group *Group) POST(pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	group.Handle(http.MethodPost, pattern, handler, middlewares...)
}

func (group *Group) PUT(pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	group.Handle(http.MethodPut, pattern, handler, middlewares...)
}

func (group *Group) PATCH(pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	group.Handle(http.MethodPatch, pattern, handler, middlewares...)
}

func (group *Group) DELETE(pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	group.Handle(http.MethodDelete, pattern, handler, middlewares...)
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
