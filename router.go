package golpher

import (
	"bytes"
	"net/http"
	"strings"
	"sync"
)

var requestPool = sync.Pool{New: func() any { return new(Request) }}
var responsePool = sync.Pool{New: func() any { return new(Response) }}

const maxPooledResponseBufferCapacity = 64 * 1024

type Router struct {
	app          *App
	routes       []route
	staticRoutes map[string]map[string]int
}

type route struct {
	method          string
	pattern         string
	trimmedPattern  string
	segments        []string
	static          bool
	handler         HandlerFunc
	middlewares     []MiddlewareFunc
	compiledHandler HandlerFunc
	rawHandler      RawHandlerFunc
}

func (r *Router) handle(method, pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	newRoute := route{
		method:         method,
		pattern:        pattern,
		trimmedPattern: strings.Trim(pattern, "/"),
		segments:       splitPath(pattern),
		static:         isStaticPattern(pattern),
		handler:        handler,
		middlewares:    append([]MiddlewareFunc(nil), middlewares...),
	}
	newRoute.rebuildHandler(r.app.middlewares)
	r.routes = append(r.routes, newRoute)
	r.registerStaticRoute(len(r.routes)-1, newRoute)
}

func (r *Router) handleRaw(method, pattern string, handler RawHandlerFunc) {
	newRoute := route{
		method:         method,
		pattern:        pattern,
		trimmedPattern: strings.Trim(pattern, "/"),
		segments:       splitPath(pattern),
		static:         isStaticPattern(pattern),
		rawHandler:     handler,
	}
	r.routes = append(r.routes, newRoute)
	r.registerStaticRoute(len(r.routes)-1, newRoute)
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
	response := acquireResponse(w, r.app.Config.DisableResponseBodyCapture)
	defer releaseResponse(response)

	if byPath := r.staticRoutes[req.Method]; byPath != nil {
		if staticIdx, ok := byPath[req.URL.Path]; ok {
			trimmedPath := strings.Trim(req.URL.Path, "/")
			if earlierIdx, params, ok := r.earlierMatch(req.Method, req.URL.Path, trimmedPath, staticIdx); ok {
				r.dispatch(response, req, params, earlierIdx)
				return
			}
			r.dispatch(response, req, nil, staticIdx)
			return
		}
	}

	var methodMismatch bool
	trimmedPath := strings.Trim(req.URL.Path, "/")

	for i := range r.routes {
		route := &r.routes[i]
		params, ok := route.match(req.URL.Path, trimmedPath)
		if !ok {
			continue
		}
		if route.method != req.Method {
			methodMismatch = true
			continue
		}

		r.dispatch(response, req, params, i)
		return
	}

	request := acquireRequest(req, nil)
	defer releaseRequest(request)
	ctx := &Context{Request: request, Response: response}
	if methodMismatch {
		response.Header().Set("Allow", r.allowedMethods(req.URL.Path, trimmedPath))
		r.app.ErrorHandler(ctx, ErrorGolpher{Code: http.StatusMethodNotAllowed, Message: "Method Not Allowed"})
		return
	}
	r.app.ErrorHandler(ctx, ErrorGolpher{Code: http.StatusNotFound, Message: "Not Found"})
}

func (r *Router) dispatch(response *Response, req *http.Request, params map[string]string, routeIndex int) {
	route := &r.routes[routeIndex]
	if route.rawHandler != nil {
		route.rawHandler(response.writer, req)
		return
	}
	request := acquireRequest(req, params)
	defer releaseRequest(request)
	handler := route.compiledHandler
	if err := handler(request, response); err != nil {
		ctx := &Context{Request: request, Response: response}
		r.app.ErrorHandler(ctx, err)
	}
}

func (r *Router) earlierMatch(method, path, trimmedPath string, beforeIndex int) (int, map[string]string, bool) {
	for i := 0; i < beforeIndex; i++ {
		route := &r.routes[i]
		if route.method != method {
			continue
		}
		params, ok := route.match(path, trimmedPath)
		if ok {
			return i, params, true
		}
	}
	return 0, nil, false
}

func (r *Router) allowedMethods(path, trimmedPath string) string {
	methods := make([]string, 0)
	for _, route := range r.routes {
		if _, ok := route.match(path, trimmedPath); ok {
			methods = append(methods, route.method)
		}
	}
	return strings.Join(methods, ", ")
}

func (r route) match(path, trimmedPath string) (map[string]string, bool) {
	if r.static {
		return nil, r.pattern == path || r.trimmedPattern == trimmedPath
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

func (r *route) rebuildHandler(appMiddlewares []MiddlewareFunc) {
	handler := r.handler
	if len(r.middlewares) > 0 {
		handler = chain(handler, r.middlewares...)
	}
	if len(appMiddlewares) > 0 {
		handler = chain(handler, appMiddlewares...)
	}
	r.compiledHandler = handler
}

func (r *Router) rebuildHandlers() {
	for i := range r.routes {
		r.routes[i].rebuildHandler(r.app.middlewares)
	}
}

func (r *Router) registerStaticRoute(index int, route route) {
	if !route.static {
		return
	}
	if r.staticRoutes == nil {
		r.staticRoutes = make(map[string]map[string]int)
	}
	byPath := r.staticRoutes[route.method]
	if byPath == nil {
		byPath = make(map[string]int)
		r.staticRoutes[route.method] = byPath
	}
	for _, path := range staticRoutePaths(route.pattern, route.trimmedPattern) {
		if _, exists := byPath[path]; !exists {
			byPath[path] = index
		}
	}
}

func staticRoutePaths(pattern, trimmedPattern string) []string {
	if trimmedPattern == "" {
		return []string{"/"}
	}
	canonical := "/" + trimmedPattern
	trailing := canonical + "/"
	if pattern == trailing {
		return []string{trailing, canonical}
	}
	return []string{canonical, trailing}
}

func acquireRequest(req *http.Request, params map[string]string) *Request {
	request := requestPool.Get().(*Request)
	request.http = req
	request.params = params
	if request.body == nil {
		request.body = &Body{}
	}
	request.body.bytes = nil
	request.body.error = nil
	request.body.loaded = false
	return request
}

func releaseRequest(request *Request) {
	if request == nil {
		return
	}
	if request.body != nil {
		request.body.bytes = nil
		request.body.error = nil
		request.body.loaded = false
	}
	request.http = nil
	request.params = nil
	requestPool.Put(request)
}

func acquireResponse(w http.ResponseWriter, disableBodyCapture ...bool) *Response {
	response := responsePool.Get().(*Response)
	response.writer = w
	response.statusCode = 0
	response.disableBodyCapture = len(disableBodyCapture) > 0 && disableBodyCapture[0]
	response.body.Reset()
	return response
}

func releaseResponse(response *Response) {
	if response == nil {
		return
	}
	response.writer = nil
	response.statusCode = 0
	response.disableBodyCapture = false
	if response.body.Cap() > maxPooledResponseBufferCapacity {
		response.body = bytes.Buffer{}
	} else {
		response.body.Reset()
	}
	responsePool.Put(response)
}

func chain(handler HandlerFunc, middlewares ...MiddlewareFunc) HandlerFunc {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}
