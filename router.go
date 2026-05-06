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

const (
	routeSegmentStatic uint8 = iota
	routeSegmentParam
	routeSegmentWildcard
)

const (
	routeKindNative uint8 = iota
	routeKindCtx
	routeKindContext
	routeKindRaw
)

type Router struct {
	app           *App
	routes        []route
	staticRoutes  map[string]map[string]int
	dynamicRoutes map[string]*routeNode
}

type route struct {
	method           string
	kind             uint8
	compiledSegments []routeSegment
	paramNames       []string
	nativeHandler    HandlerFunc
	ctxHandler       Handler
	contextHandler   ContextHandlerFunc
	middlewares      []MiddlewareFunc
	compiledHandler  HandlerFunc
	rawHandler       RawHandlerFunc
}

type routeSegment struct {
	kind  uint8
	value string
}

type routeNode struct {
	staticChildren map[string]*routeNode
	paramChild     *routeNode
	wildcardChild  *routeNode
	paramName      string
	wildcardName   string
	routeIndex     int
}

func (r *Router) handle(method, pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	compiledSegments, paramNames := compileDynamicRoute(pattern)
	newRoute := route{
		method:           method,
		kind:             routeKindNative,
		compiledSegments: compiledSegments,
		paramNames:       paramNames,
		nativeHandler:    handler,
		middlewares:      append([]MiddlewareFunc(nil), middlewares...),
	}
	newRoute.rebuildHandler(r.app.middlewares)
	r.routes = append(r.routes, newRoute)
	r.registerStaticRoute(len(r.routes)-1, method, pattern)
	r.registerDynamicRoute(len(r.routes)-1, method, newRoute.compiledSegments)
}

func (r *Router) handleCtx(method, pattern string, handler Handler, middlewares ...MiddlewareFunc) {
	compiledSegments, paramNames := compileDynamicRoute(pattern)
	newRoute := route{
		method:           method,
		kind:             routeKindCtx,
		compiledSegments: compiledSegments,
		paramNames:       paramNames,
		ctxHandler:       handler,
		middlewares:      append([]MiddlewareFunc(nil), middlewares...),
	}
	newRoute.rebuildHandler(r.app.middlewares)
	r.routes = append(r.routes, newRoute)
	r.registerStaticRoute(len(r.routes)-1, method, pattern)
	r.registerDynamicRoute(len(r.routes)-1, method, newRoute.compiledSegments)
}

func (r *Router) handleContext(method, pattern string, handler ContextHandlerFunc, middlewares ...MiddlewareFunc) {
	compiledSegments, paramNames := compileDynamicRoute(pattern)
	newRoute := route{
		method:           method,
		kind:             routeKindContext,
		compiledSegments: compiledSegments,
		paramNames:       paramNames,
		contextHandler:   handler,
		middlewares:      append([]MiddlewareFunc(nil), middlewares...),
	}
	newRoute.rebuildHandler(r.app.middlewares)
	r.routes = append(r.routes, newRoute)
	r.registerStaticRoute(len(r.routes)-1, method, pattern)
	r.registerDynamicRoute(len(r.routes)-1, method, newRoute.compiledSegments)
}

func (r *Router) handleRaw(method, pattern string, handler RawHandlerFunc) {
	compiledSegments, paramNames := compileDynamicRoute(pattern)
	newRoute := route{
		method:           method,
		kind:             routeKindRaw,
		compiledSegments: compiledSegments,
		paramNames:       paramNames,
		rawHandler:       handler,
	}
	r.routes = append(r.routes, newRoute)
	r.registerStaticRoute(len(r.routes)-1, method, pattern)
	r.registerDynamicRoute(len(r.routes)-1, method, newRoute.compiledSegments)
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
			r.dispatch(response, req, nil, staticIdx)
			return
		}
	}

	trimmedPath := strings.Trim(req.URL.Path, "/")
	request := acquireRequest(req, nil)
	defer releaseRequest(request)
	if tree := r.dynamicRoutes[req.Method]; tree != nil {
		if routeIndex, ok := tree.match(trimmedPath, request); ok {
			request.paramNames = r.routes[routeIndex].paramNames
			r.dispatchRequest(response, request, routeIndex)
			return
		}
	}

	methodMismatch := r.pathMatchesAnyMethod(req.Method, req.URL.Path, trimmedPath)
	if methodMismatch {
		ctx := &Context{Request: request, Response: response}
		response.Header().Set("Allow", r.allowedMethods(req.Method, req.URL.Path, trimmedPath))
		r.app.ErrorHandler(ctx, ErrorGolpher{Code: http.StatusMethodNotAllowed, Message: "Method Not Allowed"})
		return
	}

	ctx := &Context{Request: request, Response: response}
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
	r.dispatchRequest(response, request, routeIndex)
}

func (r *Router) dispatchRequest(response *Response, request *Request, routeIndex int) {
	route := &r.routes[routeIndex]
	if route.kind == routeKindRaw {
		route.rawHandler(response.writer, request.http)
		return
	}
	var err error
	if route.compiledHandler != nil {
		err = route.compiledHandler(request, response)
	} else {
		switch route.kind {
		case routeKindCtx:
			ctx := request.acquireCtx(response)
			err = route.ctxHandler(ctx)
		case routeKindContext:
			ctx := request.acquireCtx(response)
			err = route.contextHandler(ctx, request, response)
		default:
			err = route.nativeHandler(request, response)
		}
	}
	if err != nil {
		ctx := &Context{Request: request, Response: response}
		r.app.ErrorHandler(ctx, err)
	}
}

func (r *Router) allowedMethods(currentMethod, path, trimmedPath string) string {
	methods := make([]string, 0)
	seen := make(map[string]struct{})
	for i := range r.routes {
		route := &r.routes[i]
		if route.method == currentMethod {
			continue
		}
		if _, exists := seen[route.method]; exists {
			continue
		}
		if route.matchesStaticIndex(r.staticRoutes, i, path) || route.matchInto(path, trimmedPath, &Request{}) {
			methods = append(methods, route.method)
			seen[route.method] = struct{}{}
		}
	}
	return strings.Join(methods, ", ")
}

func (r *Router) pathMatchesAnyMethod(currentMethod, path, trimmedPath string) bool {
	for i := range r.routes {
		route := &r.routes[i]
		if route.method == currentMethod {
			continue
		}
		if route.matchesStaticIndex(r.staticRoutes, i, path) || route.matchInto(path, trimmedPath, &Request{}) {
			return true
		}
	}
	return false
}

func (r route) match(path, trimmedPath string) (map[string]string, bool) {
	var request Request
	if !r.matchInto(path, trimmedPath, &request) {
		return nil, false
	}
	if len(request.paramNames) == 0 {
		return nil, true
	}
	params := make(map[string]string, len(request.paramNames))
	for i, name := range request.paramNames {
		params[name] = request.paramValues[i]
	}
	return params, true
}

func (r route) matchInto(path, trimmedPath string, request *Request) bool {
	request.paramNames = nil
	request.paramValues = request.paramValues[:0]
	request.params = nil
	if !r.isDynamic() {
		return false
	}
	if trimmedPath == "" {
		if len(r.compiledSegments) == 1 && r.compiledSegments[0].kind == routeSegmentWildcard {
			request.paramNames = r.paramNames
			request.paramValues = append(request.paramValues, "")
			return true
		}
		return false
	}

	request.paramNames = r.paramNames
	position := 0
	for i, segment := range r.compiledSegments {
		if position > len(trimmedPath) {
			return false
		}
		next := nextSegmentEnd(trimmedPath, position)
		pathSegment := trimmedPath[position:next]
		switch segment.kind {
		case routeSegmentParam:
			request.paramValues = append(request.paramValues, pathSegment)
		case routeSegmentWildcard:
			request.paramValues = append(request.paramValues, trimmedPath[position:])
			return true
		default:
			if segment.value != pathSegment {
				return false
			}
		}
		if i == len(r.compiledSegments)-1 {
			return next == len(trimmedPath)
		}
		position = next + 1
	}
	return trimmedPath == ""
}

func (r route) isDynamic() bool {
	return len(r.compiledSegments) > 0
}

func (r *route) matchesStaticIndex(staticRoutes map[string]map[string]int, routeIndex int, path string) bool {
	if r.isDynamic() {
		return false
	}
	byPath := staticRoutes[r.method]
	if byPath == nil {
		return false
	}
	idx, ok := byPath[path]
	return ok && idx == routeIndex
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

func compileDynamicRoute(pattern string) ([]routeSegment, []string) {
	if isStaticPattern(pattern) {
		return nil, nil
	}
	segments := compileRouteSegments(pattern)
	return segments, paramNamesFromSegments(segments)
}

func routeParamNames(pattern string) []string {
	return paramNamesFromSegments(compileRouteSegments(pattern))
}

func paramNamesFromSegments(segments []routeSegment) []string {
	var names []string
	for _, segment := range segments {
		if segment.kind == routeSegmentParam || segment.kind == routeSegmentWildcard {
			names = append(names, segment.value)
		}
	}
	return names
}

func compileRouteSegments(pattern string) []routeSegment {
	parts := splitPath(pattern)
	if len(parts) == 0 {
		return nil
	}
	segments := make([]routeSegment, len(parts))
	for i, part := range parts {
		if strings.HasPrefix(part, ":") {
			segments[i] = routeSegment{kind: routeSegmentParam, value: part[1:]}
			continue
		}
		if strings.HasPrefix(part, "*") {
			segments[i] = routeSegment{kind: routeSegmentWildcard, value: part[1:]}
			continue
		}
		segments[i] = routeSegment{kind: routeSegmentStatic, value: part}
	}
	return segments
}

func nextSegmentEnd(path string, start int) int {
	for i := start; i < len(path); i++ {
		if path[i] == '/' {
			return i
		}
	}
	return len(path)
}

func (r *route) rebuildHandler(appMiddlewares []MiddlewareFunc) {
	if r.kind == routeKindRaw {
		r.compiledHandler = nil
		return
	}
	var handler HandlerFunc
	switch r.kind {
	case routeKindCtx:
		handler = adaptCtxHandler(r.ctxHandler)
	case routeKindContext:
		handler = adaptContextHandler(r.contextHandler)
	default:
		handler = r.nativeHandler
	}
	if len(r.middlewares) > 0 {
		handler = chain(handler, r.middlewares...)
	}
	if len(appMiddlewares) > 0 {
		handler = chain(handler, appMiddlewares...)
	}
	if len(r.middlewares) == 0 && len(appMiddlewares) == 0 {
		r.compiledHandler = nil
		return
	}
	r.compiledHandler = handler
}

func (r *Router) rebuildHandlers() {
	for i := range r.routes {
		r.routes[i].rebuildHandler(r.app.middlewares)
	}
}

func (r *Router) registerStaticRoute(index int, method, pattern string) {
	if !isStaticPattern(pattern) {
		return
	}
	if r.staticRoutes == nil {
		r.staticRoutes = make(map[string]map[string]int)
	}
	byPath := r.staticRoutes[method]
	if byPath == nil {
		byPath = make(map[string]int)
		r.staticRoutes[method] = byPath
	}
	for _, path := range staticRoutePaths(pattern) {
		if _, exists := byPath[path]; !exists {
			byPath[path] = index
		}
	}
}

func (r *Router) registerDynamicRoute(index int, method string, segments []routeSegment) {
	if len(segments) == 0 {
		return
	}
	if r.dynamicRoutes == nil {
		r.dynamicRoutes = make(map[string]*routeNode)
	}
	root := r.dynamicRoutes[method]
	if root == nil {
		root = &routeNode{routeIndex: -1}
		r.dynamicRoutes[method] = root
	}
	node := root
	for _, segment := range segments {
		switch segment.kind {
		case routeSegmentParam:
			if node.paramChild == nil {
				node.paramChild = &routeNode{routeIndex: -1}
			}
			node.paramName = segment.value
			node = node.paramChild
		case routeSegmentWildcard:
			if node.wildcardChild == nil {
				node.wildcardChild = &routeNode{routeIndex: -1}
			}
			node.wildcardName = segment.value
			node = node.wildcardChild
		default:
			if node.staticChildren == nil {
				node.staticChildren = make(map[string]*routeNode)
			}
			child := node.staticChildren[segment.value]
			if child == nil {
				child = &routeNode{routeIndex: -1}
				node.staticChildren[segment.value] = child
			}
			node = child
		}
	}
	if node.routeIndex < 0 {
		node.routeIndex = index
	}
}

func (n *routeNode) match(trimmedPath string, request *Request) (int, bool) {
	request.paramNames = nil
	request.paramValues = request.paramValues[:0]
	request.params = nil
	if trimmedPath == "" {
		if n.wildcardChild != nil && n.wildcardChild.routeIndex >= 0 {
			request.paramValues = append(request.paramValues, "")
			return n.wildcardChild.routeIndex, true
		}
		return n.routeIndex, n.routeIndex >= 0
	}
	return n.matchFrom(trimmedPath, 0, request)
}

func (n *routeNode) matchFrom(path string, position int, request *Request) (int, bool) {
	next := nextSegmentEnd(path, position)
	segment := path[position:next]
	last := next == len(path)
	if child := n.staticChildren[segment]; child != nil {
		if last {
			if child.routeIndex >= 0 {
				return child.routeIndex, true
			}
		} else if routeIndex, ok := child.matchFrom(path, next+1, request); ok {
			return routeIndex, true
		}
	}
	if n.paramChild != nil && segment != "" {
		valueCount := len(request.paramValues)
		request.paramValues = append(request.paramValues, segment)
		if last {
			if n.paramChild.routeIndex >= 0 {
				return n.paramChild.routeIndex, true
			}
		} else if routeIndex, ok := n.paramChild.matchFrom(path, next+1, request); ok {
			return routeIndex, true
		}
		request.paramValues = request.paramValues[:valueCount]
	}
	if n.wildcardChild != nil && n.wildcardChild.routeIndex >= 0 {
		request.paramValues = append(request.paramValues, path[position:])
		return n.wildcardChild.routeIndex, true
	}
	return 0, false
}

func staticRoutePaths(pattern string) []string {
	trimmedPattern := strings.Trim(pattern, "/")
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
	request.paramNames = nil
	request.paramValues = request.paramValues[:0]
	request.ctx.request = nil
	request.ctx.response = nil
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
