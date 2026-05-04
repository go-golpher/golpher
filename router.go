package golpher

import (
	"net/http"
	"strings"
	"sync"
)

var requestPool = sync.Pool{New: func() any { return new(Request) }}
var responsePool = sync.Pool{New: func() any { return new(Response) }}

type Router struct {
	app    *App
	routes []route
}

type route struct {
	method      string
	pattern     string
	segments    []string
	static      bool
	handler     HandlerFunc
	middlewares []MiddlewareFunc
}

func (r *Router) handle(method, pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	r.routes = append(r.routes, route{
		method:      method,
		pattern:     pattern,
		segments:    splitPath(pattern),
		static:      isStaticPattern(pattern),
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
	response := acquireResponse(w)
	defer releaseResponse(response)
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

		request := acquireRequest(req, params)
		defer releaseRequest(request)
		handler := route.handler
		if len(r.app.middlewares) > 0 || len(route.middlewares) > 0 {
			handler = chain(route.handler, append(append([]MiddlewareFunc(nil), r.app.middlewares...), route.middlewares...)...)
		}
		if err := handler(request, response); err != nil {
			ctx := &Context{Request: request, Response: response}
			r.app.ErrorHandler(ctx, err)
		}
		return
	}

	request := acquireRequest(req, nil)
	defer releaseRequest(request)
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
	if r.static {
		return nil, r.pattern == path || strings.Trim(r.pattern, "/") == strings.Trim(path, "/")
	}

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

func isStaticPattern(pattern string) bool {
	return !strings.ContainsAny(pattern, ":*")
}

func acquireRequest(req *http.Request, params map[string]string) *Request {
	request := requestPool.Get().(*Request)
	request.http = req
	request.params = params
	request.body = nil
	return request
}

func releaseRequest(request *Request) {
	if request == nil {
		return
	}
	if request.body != nil {
		request.body.bytes = nil
		request.body.error = nil
	}
	request.http = nil
	request.params = nil
	request.body = nil
	requestPool.Put(request)
}

func acquireResponse(w http.ResponseWriter) *Response {
	response := responsePool.Get().(*Response)
	response.writer = w
	response.statusCode = 0
	response.body.Reset()
	return response
}

func releaseResponse(response *Response) {
	if response == nil {
		return
	}
	response.writer = nil
	response.statusCode = 0
	response.body.Reset()
	responsePool.Put(response)
}

func chain(handler HandlerFunc, middlewares ...MiddlewareFunc) HandlerFunc {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}
