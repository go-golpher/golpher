package golpher

import "net/http"

// HandlerFunc is the exported handler signature. Every route, verb
// shorthand, middleware chain, and adapter targets this type.
type HandlerFunc func(*Request, *Response) error

// MiddlewareFunc wraps a HandlerFunc to add pre/post processing.
type MiddlewareFunc func(HandlerFunc) HandlerFunc

// FromHTTPHandler adapts a standard-library http.Handler into a
// Golpher HandlerFunc. Writes are tracked through the Response.
func FromHTTPHandler(handler http.Handler) HandlerFunc {
	return func(req *Request, res *Response) error {
		handler.ServeHTTP(res.Raw(), req.http)
		return nil
	}
}

// FromHTTPHandlerFunc adapts a standard-library http.HandlerFunc into a
// Golpher HandlerFunc.
func FromHTTPHandlerFunc(handler http.HandlerFunc) HandlerFunc {
	return FromHTTPHandler(handler)
}
