package golpher

import (
	"bytes"
	"io"
	"log"
	"net/http"
)

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

func BodyLimit(maxBytes int64) MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(req *Request, res *Response) error {
			if maxBytes >= 0 && req.http.ContentLength > maxBytes {
				return ErrorGolpher{Code: http.StatusRequestEntityTooLarge, Message: http.StatusText(http.StatusRequestEntityTooLarge)}
			}
			if req.http.Body != nil {
				data, err := io.ReadAll(io.LimitReader(req.http.Body, maxBytes+1))
				if err != nil {
					return err
				}
				if int64(len(data)) > maxBytes {
					return ErrorGolpher{Code: http.StatusRequestEntityTooLarge, Message: http.StatusText(http.StatusRequestEntityTooLarge)}
				}
				req.body = &Body{bytes: data}
				req.http.Body = io.NopCloser(bytes.NewReader(data))
			}
			return next(req, res)
		}
	}
}
