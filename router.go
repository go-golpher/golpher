package golpher

import (
	"net/http"
	"strings"
)

type Router struct {
	app    *App
	routes []route
}

type route struct {
	method      string
	pattern     string
	segments    []string
	handler     HandlerFunc
	middlewares []MiddlewareFunc
}

func (r *Router) handle(method, pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	r.routes = append(r.routes, route{
		method:      method,
		pattern:     pattern,
		segments:    splitPath(pattern),
		handler:     handler,
		middlewares: append([]MiddlewareFunc(nil), middlewares...),
	})
}

func (r *Router) GET(pattern string, handler HandlerFunc) {
	r.handle(http.MethodGet, pattern, handler)
}

func (r *Router) POST(pattern string, handler HandlerFunc) {
	r.handle(http.MethodPost, pattern, handler)
}

func (r *Router) PUT(pattern string, handler HandlerFunc) {
	r.handle(http.MethodPut, pattern, handler)
}

func (r *Router) DELETE(pattern string, handler HandlerFunc) {
	r.handle(http.MethodDelete, pattern, handler)
}

func (r *Router) PATCH(pattern string, handler HandlerFunc) {
	r.handle(http.MethodPatch, pattern, handler)
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	response := &Response{writer: w}
	var methodMismatch bool

	for _, route := range r.routes {
		params, ok := route.match(req.URL.Path)
		if !ok {
			continue
		}
		if route.method != req.Method {
			methodMismatch = true
			continue
		}

		request := &Request{http: req, params: params}
		ctx := &Context{Request: request, Response: response}
		handler := chain(route.handler, append(append([]MiddlewareFunc(nil), r.app.middlewares...), route.middlewares...)...)
		if err := handler(request, response); err != nil {
			r.app.ErrorHandler(ctx, err)
		}
		return
	}

	request := &Request{http: req}
	ctx := &Context{Request: request, Response: response}
	if methodMismatch {
		response.Header().Set("Allow", r.allowedMethods(req.URL.Path))
		r.app.ErrorHandler(ctx, ErrorGolpher{Code: http.StatusMethodNotAllowed, Message: "Method Not Allowed"})
		return
	}
	r.app.ErrorHandler(ctx, ErrorGolpher{Code: http.StatusNotFound, Message: "Not Found"})
}

func (r *Router) allowedMethods(path string) string {
	methods := make([]string, 0)
	for _, route := range r.routes {
		if _, ok := route.match(path); ok {
			methods = append(methods, route.method)
		}
	}
	return strings.Join(methods, ", ")
}

func (r route) match(path string) (map[string]string, bool) {
	pathSegments := splitPath(path)
	if len(r.segments) != len(pathSegments) {
		return nil, false
	}
	params := make(map[string]string)
	for i, segment := range r.segments {
		if strings.HasPrefix(segment, ":") {
			params[strings.TrimPrefix(segment, ":")] = pathSegments[i]
			continue
		}
		if strings.HasPrefix(segment, "*") {
			params[strings.TrimPrefix(segment, "*")] = strings.Join(pathSegments[i:], "/")
			return params, true
		}
		if segment != pathSegments[i] {
			return nil, false
		}
	}
	return params, true
}

func splitPath(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

func chain(handler HandlerFunc, middlewares ...MiddlewareFunc) HandlerFunc {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}
