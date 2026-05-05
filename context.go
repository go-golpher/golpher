package golpher

import "net/http"

type Context struct {
	Request  *Request
	Response *Response
}

type HandlerFunc func(Request *Request, Response *Response) error

type RawHandlerFunc func(http.ResponseWriter, *http.Request)

type MiddlewareFunc func(HandlerFunc) HandlerFunc
