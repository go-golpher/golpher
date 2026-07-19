package golpher

import (
	"log"
	"net/http"
)

// Recover returns middleware that converts panics into 500 Internal
// Server Error responses. The original panic value is logged.
func Recover() MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(req *Request, res *Response) (err error) {
			defer func() {
				if recovered := recover(); recovered != nil {
					log.Printf("golpher: recovered panic: %v", recovered)
					err = ErrorGolpher{Code: http.StatusInternalServerError, Message: http.StatusText(http.StatusInternalServerError)}
				}
			}()
			return next(req, res)
		}
	}
}

// BodyLimit returns middleware that overrides the per-request body
// size limit. Enforcement is lazy: the body is not read or wrapped
// until the handler first accesses it via http.MaxBytesReader.
//
// Semantics:
//
//	BodyLimit(n < 0)  → unlimited for this route
//	BodyLimit(0)      → use the app-level default
//	BodyLimit(n > 0)  → enforce at most n bytes
func BodyLimit(maxBytes int64) MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(req *Request, res *Response) error {
			req.bodyLimitOverride = maxBytes
			return next(req, res)
		}
	}
}
