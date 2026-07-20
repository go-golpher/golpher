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

type Router struct {
	app           *App
	routes        []route
	staticRoutes  map[string]map[string]int // method → path → route index
	dynamicRoutes map[string]*routeNode     // method → trie root
}

type route struct {
	method           string
	compiledSegments []routeSegment
	paramNames       []string
	nativeHandler    HandlerFunc
	middlewares      []MiddlewareFunc
	compiledHandler  HandlerFunc
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
	validateMethod(method)
	validatePattern(pattern)
	if handler == nil {
		panic("golpher: nil handler")
	}

	compiledSegments, paramNames := compileRouteSegmentsResult(pattern)
	canonical := canonicalPattern(pattern)

	for i := range r.routes {
		existing := &r.routes[i]
		if existing.method == method && canonicalPattern(existing.canonical()) == canonical {
			panic("golpher: duplicate route: " + method + " " + canonical)
		}
	}

	newRoute := route{
		method:           method,
		compiledSegments: compiledSegments,
		paramNames:       paramNames,
		nativeHandler:    handler,
		middlewares:      append([]MiddlewareFunc(nil), middlewares...),
	}
	newRoute.rebuildHandler(r.app.middlewares)
	r.routes = append(r.routes, newRoute)
	idx := len(r.routes) - 1

	r.registerStaticRoute(idx, method, pattern)
	r.registerDynamicRoute(idx, method, newRoute.compiledSegments)
}

func (r route) canonical() string {
	if len(r.compiledSegments) == 0 {
		return "" // handled by registerStaticRoute
	}
	parts := make([]string, len(r.compiledSegments))
	for i, seg := range r.compiledSegments {
		switch seg.kind {
		case routeSegmentParam:
			parts[i] = ":" + seg.value
		case routeSegmentWildcard:
			parts[i] = "*" + seg.value
		default:
			parts[i] = seg.value
		}
	}
	return "/" + strings.Join(parts, "/")
}

// validateMethod checks that method is a valid RFC 9110 token.
func validateMethod(method string) {
	if len(method) == 0 {
		panic("golpher: invalid method: empty")
	}
	for i := 0; i < len(method); i++ {
		if !isTokenByte(method[i]) {
			panic("golpher: invalid method: " + method)
		}
	}
}

// isTokenByte returns true for RFC 9110 tchar characters.
func isTokenByte(c byte) bool {
	if c >= 'a' && c <= 'z' {
		return true
	}
	if c >= 'A' && c <= 'Z' {
		return true
	}
	if c >= '0' && c <= '9' {
		return true
	}
	switch c {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	}
	return false
}

// validatePattern checks pattern syntax. Must start with '/'.
func validatePattern(pattern string) {
	if len(pattern) == 0 || pattern[0] != '/' {
		panic("golpher: invalid pattern: must start with /")
	}
	parts := splitPattern(pattern)
	if len(parts) == 0 {
		return // root "/" is valid
	}
	seenParams := make(map[string]bool)
	for i, part := range parts {
		if strings.HasPrefix(part, ":") {
			name := part[1:]
			if name == "" {
				panic("golpher: invalid pattern: empty param name in " + pattern)
			}
			if !isParamName(name) {
				panic("golpher: invalid pattern: invalid param name :" + name + " in " + pattern)
			}
			if seenParams[name] {
				panic("golpher: invalid pattern: duplicate param name :" + name + " in " + pattern)
			}
			seenParams[name] = true
			continue
		}
		if strings.HasPrefix(part, "*") {
			name := part[1:]
			if name == "" {
				panic("golpher: invalid pattern: empty wildcard name in " + pattern)
			}
			if !isParamName(name) {
				panic("golpher: invalid pattern: invalid wildcard name *" + name + " in " + pattern)
			}
			if i != len(parts)-1 {
				panic("golpher: invalid pattern: wildcard must be final segment in " + pattern)
			}
			continue
		}
	}
}

// isParamName checks that name matches [A-Za-z_][A-Za-z0-9_]*.
func isParamName(name string) bool {
	if len(name) == 0 {
		return false
	}
	first := name[0]
	if !isASCIIAlpha(first) && first != '_' {
		return false
	}
	for i := 1; i < len(name); i++ {
		c := name[i]
		if !isASCIIAlpha(c) && !isASCIIDigit(c) && c != '_' {
			return false
		}
	}
	return true
}

func isASCIIAlpha(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
}

func isASCIIDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func splitPattern(pattern string) []string {
	trimmed := strings.Trim(pattern, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func canonicalPattern(pattern string) string {
	trimmed := strings.Trim(pattern, "/")
	if trimmed == "" {
		return "/"
	}
	return "/" + trimmed
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	response := acquireResponse(w, r.app.config.EnableBodyCapture)
	defer releaseResponse(response)

	if byPath := r.staticRoutes[req.Method]; byPath != nil {
		if staticIdx, ok := byPath[req.URL.Path]; ok {
			r.dispatch(response, req, staticIdx)
			return
		}
	}

	trimmedPath := strings.Trim(req.URL.Path, "/")
	request := acquireRequest(req, response, r.app.config.MaxRequestBodyBytes)
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
		response.Header().Set("Allow", r.allowedMethods(req.Method, req.URL.Path, trimmedPath))
		r.app.reportError(request, response,
			ErrorGolpher{Code: http.StatusMethodNotAllowed, Message: "Method Not Allowed"})
		return
	}

	r.app.reportError(request, response,
		ErrorGolpher{Code: http.StatusNotFound, Message: "Not Found"})
}

func (r *Router) dispatchQUERYGuard(req *http.Request, request *Request, response *Response) bool {
	if req.Header.Get("Content-Type") == "" {
		r.app.reportError(request, response,
			ErrorGolpher{Code: http.StatusBadRequest, Message: "Content-Type header required for QUERY"})
		return false
	}
	return true
}

func (r *Router) dispatch(response *Response, req *http.Request, routeIndex int) {
	request := acquireRequest(req, response, r.app.config.MaxRequestBodyBytes)
	defer releaseRequest(request)
	r.dispatchRequest(response, request, routeIndex)
}

func (r *Router) dispatchRequest(response *Response, request *Request, routeIndex int) {
	route := &r.routes[routeIndex]

	if route.method == MethodQuery {
		if !r.dispatchQUERYGuard(request.http, request, response) {
			return
		}
	}

	var err error
	if route.compiledHandler != nil {
		err = route.compiledHandler(request, response)
	} else {
		err = route.nativeHandler(request, response)
	}

	if err != nil {
		if r.app.handleMaxBytesError(request, response, err) {
			return
		}
		r.app.reportError(request, response, err)
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

func (rt route) matchInto(path, trimmedPath string, request *Request) bool {
	request.paramNames = nil
	request.paramValues = request.paramValues[:0]
	request.params = nil
	if !rt.isDynamic() {
		return false
	}
	if trimmedPath == "" {
		if len(rt.compiledSegments) == 1 && rt.compiledSegments[0].kind == routeSegmentWildcard {
			request.paramNames = rt.paramNames
			request.paramValues = append(request.paramValues, "")
			return true
		}
		return false
	}

	request.paramNames = rt.paramNames
	position := 0
	for i, segment := range rt.compiledSegments {
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
		if i == len(rt.compiledSegments)-1 {
			return next == len(trimmedPath)
		}
		position = next + 1
	}
	return trimmedPath == ""
}

func (rt route) isDynamic() bool {
	return len(rt.compiledSegments) > 0
}

func (rt *route) matchesStaticIndex(staticRoutes map[string]map[string]int, routeIndex int, path string) bool {
	if rt.isDynamic() {
		return false
	}
	byPath := staticRoutes[rt.method]
	if byPath == nil {
		return false
	}
	idx, ok := byPath[path]
	return ok && idx == routeIndex
}

func nextSegmentEnd(path string, start int) int {
	for i := start; i < len(path); i++ {
		if path[i] == '/' {
			return i
		}
	}
	return len(path)
}

func compileRouteSegmentsResult(pattern string) ([]routeSegment, []string) {
	trimmed := strings.Trim(pattern, "/")
	if trimmed == "" {
		return nil, nil
	}
	parts := strings.Split(trimmed, "/")
	segments := make([]routeSegment, len(parts))
	var names []string
	for i, part := range parts {
		switch {
		case strings.HasPrefix(part, ":"):
			name := part[1:]
			segments[i] = routeSegment{kind: routeSegmentParam, value: name}
			names = append(names, name)
		case strings.HasPrefix(part, "*"):
			name := part[1:]
			segments[i] = routeSegment{kind: routeSegmentWildcard, value: name}
			names = append(names, name)
		default:
			segments[i] = routeSegment{kind: routeSegmentStatic, value: part}
		}
	}
	return segments, names
}

func (r *Router) registerStaticRoute(index int, method, pattern string) {
	if strings.ContainsAny(pattern, ":*") {
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
	trimmed := strings.Trim(pattern, "/")
	canonical := "/" + trimmed
	trailing := canonical + "/"
	if !strings.HasSuffix(pattern, "/") {
		byPath[canonical] = index
		if _, exists := byPath[trailing]; !exists {
			byPath[trailing] = index
		}
	} else {
		byPath[trailing] = index
		if _, exists := byPath[canonical]; !exists {
			byPath[canonical] = index
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
			if node.wildcardChild != nil {
				panic("golpher: conflicting route: :param vs *wildcard at same position")
			}
			if node.paramChild != nil && node.paramName != "" && node.paramName != segment.value {
				panic("golpher: conflicting route: :" + node.paramName + " vs :" + segment.value)
			}
			if node.paramChild == nil {
				node.paramChild = &routeNode{routeIndex: -1}
			}
			node.paramName = segment.value
			node = node.paramChild
		case routeSegmentWildcard:
			if node.paramChild != nil {
				panic("golpher: conflicting route: :param vs *wildcard at same position")
			}
			if node.wildcardChild != nil && node.wildcardName != "" && node.wildcardName != segment.value {
				panic("golpher: conflicting route: *" + node.wildcardName + " vs *" + segment.value)
			}
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

func (rt *route) rebuildHandler(appMiddlewares []MiddlewareFunc) {
	handler := rt.nativeHandler
	if len(rt.middlewares) > 0 {
		handler = chain(handler, rt.middlewares...)
	}
	if len(appMiddlewares) > 0 {
		handler = chain(handler, appMiddlewares...)
	}
	if len(rt.middlewares) == 0 && len(appMiddlewares) == 0 {
		rt.compiledHandler = nil
		return
	}
	rt.compiledHandler = handler
}

func (r *Router) rebuildHandlers() {
	for i := range r.routes {
		r.routes[i].rebuildHandler(r.app.middlewares)
	}
}

func chain(handler HandlerFunc, middlewares ...MiddlewareFunc) HandlerFunc {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

func acquireRequest(req *http.Request, response *Response, appBodyLimit int64) *Request {
	request := requestPool.Get().(*Request)
	request.http = req
	request.params = nil
	request.paramNames = nil
	request.paramValues = request.paramValues[:0]
	request.response = response
	request.bodyData = nil
	request.bodyErr = nil
	request.bodyRead = false
	request.bodyWrapped = false
	request.appBodyLimit = appBodyLimit
	request.bodyLimitOverride = 0
	return request
}

func releaseRequest(request *Request) {
	if request == nil {
		return
	}
	request.bodyData = nil
	request.bodyErr = nil
	request.bodyRead = false
	request.bodyWrapped = false
	request.http = nil
	request.params = nil
	request.paramNames = nil
	request.paramValues = request.paramValues[:0]
	request.response = nil
	request.appBodyLimit = 0
	request.bodyLimitOverride = 0
	requestPool.Put(request)
}

func acquireResponse(w http.ResponseWriter, captureEnabled bool) *Response {
	response := responsePool.Get().(*Response)
	tw := &trackingResponseWriter{w: w, res: response}
	response.writer = tw
	response.status = 0
	response.bytesWritten = 0
	response.committed = false
	response.captureEnabled = captureEnabled
	response.body.Reset()
	return response
}

func releaseResponse(response *Response) {
	if response == nil {
		return
	}
	response.writer = nil
	response.status = 0
	response.bytesWritten = 0
	response.committed = false
	response.captureEnabled = false
	if response.body.Cap() > maxPooledResponseBufferCapacity {
		response.body = bytes.Buffer{}
	} else {
		response.body.Reset()
	}
	responsePool.Put(response)
}
