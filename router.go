package golpher

import "net/http"

type Router struct {
	mux *http.ServeMux
	app *App
}

func (r *Router) handle(method, pattern string, handler HandlerFunc) {
	r.mux.HandleFunc(pattern, func(w http.ResponseWriter, req *http.Request) {
		request := &Request{http: req}
		response := &Response{writer: w}
		ctx := &Context{
			Request:  request,
			Response: response,
		}
		if req.Method != method {
			r.app.ErrorHandler(ctx, ErrorGolpher{Code: 405, Message: "Method Not Allowed"})
			return
		}
		if err := handler(request, response); err != nil {
			r.app.ErrorHandler(ctx, err)
			return
		}
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
