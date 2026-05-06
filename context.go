package golpher

import "net/http"

type Context struct {
	Request  *Request
	Response *Response
}

type Ctx struct {
	request  *Request
	response *Response
}

type HandlerFunc func(Request *Request, Response *Response) error

type Handler func(*Ctx) error

type ContextHandlerFunc func(*Ctx, *Request, *Response) error

type RawHandlerFunc func(http.ResponseWriter, *http.Request)

type MiddlewareFunc func(HandlerFunc) HandlerFunc

func (ctx *Ctx) RequestRef() *Request {
	return ctx.request
}

func (ctx *Ctx) ResponseRef() *Response {
	return ctx.response
}

func (ctx *Ctx) Param(name string) string {
	return ctx.request.Param(name)
}

func (ctx *Ctx) RawRequest() *http.Request {
	return ctx.request.Raw()
}

func (ctx *Ctx) RawResponse() http.ResponseWriter {
	return ctx.response.Raw()
}

func (ctx *Ctx) Status(code int) *Ctx {
	ctx.response.Status(code)
	return ctx
}

func (ctx *Ctx) String(body string) error {
	return ctx.response.String(body)
}

func (ctx *Ctx) Send(body []byte) error {
	return ctx.response.Send(body)
}

func (ctx *Ctx) Bytes(status int, contentType string, body []byte) error {
	return ctx.response.Bytes(status, contentType, body)
}

func (ctx *Ctx) JSONBytes(body []byte) error {
	return ctx.response.JSONBytes(body)
}

func adaptCtxHandler(handler Handler) HandlerFunc {
	return func(req *Request, res *Response) error {
		return handler(req.acquireCtx(res))
	}
}

func adaptContextHandler(handler ContextHandlerFunc) HandlerFunc {
	return func(req *Request, res *Response) error {
		return handler(req.acquireCtx(res), req, res)
	}
}

func (request *Request) acquireCtx(response *Response) *Ctx {
	request.ctx.request = request
	request.ctx.response = response
	return &request.ctx
}
