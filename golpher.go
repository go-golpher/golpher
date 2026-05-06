package golpher

import (
	"context"
	"net/http"
	"time"
)

type App struct {
	ErrorHandler ErrorHandlerFunc
	Router       *Router
	Config       AppConfig
	middlewares  []MiddlewareFunc
}

type AppConfig struct {
	ErrorHandler      ErrorHandlerFunc
	Port              int
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxHeaderBytes    int
	// DisableResponseBodyCapture skips storing bytes written through Response.Send
	// and Response.String. It is intended for latency-sensitive services that do
	// not inspect Response.Body() from middleware or tests.
	DisableResponseBodyCapture bool
	// DisableBanner skips the startup banner printed by Listen.
	DisableBanner bool
}

func New(configs ...AppConfig) *App {
	var cfg AppConfig
	if len(configs) > 0 {
		cfg = configs[0]
	}
	if cfg.ErrorHandler == nil {
		cfg.ErrorHandler = defaultErrorHandler
	}
	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	if cfg.ReadHeaderTimeout == 0 {
		cfg.ReadHeaderTimeout = 5 * time.Second
	}
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 10 * time.Second
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 30 * time.Second
	}
	if cfg.IdleTimeout == 0 {
		cfg.IdleTimeout = 120 * time.Second
	}
	app := &App{
		ErrorHandler: cfg.ErrorHandler,
		Config:       cfg,
	}
	app.Router = &Router{
		app: app,
	}
	return app
}

func (app *App) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	app.Router.ServeHTTP(w, req)
}

func (app *App) Use(middlewares ...MiddlewareFunc) {
	app.middlewares = append(app.middlewares, middlewares...)
	app.Router.rebuildHandlers()
}

func (app *App) UseHTTP(middlewares ...func(http.Handler) http.Handler) {
	for _, middleware := range middlewares {
		mw := middleware
		app.Use(func(next HandlerFunc) HandlerFunc {
			return func(req *Request, res *Response) error {
				var handlerErr error
				h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					request := &Request{http: r, params: req.params, paramNames: req.paramNames, paramValues: req.paramValues}
					response := &Response{writer: w, disableBodyCapture: app.Config.DisableResponseBodyCapture}
					handlerErr = next(request, response)
					if handlerErr != nil {
						app.ErrorHandler(&Context{Request: request, Response: response}, handlerErr)
						handlerErr = nil
					}
				}))
				h.ServeHTTP(res.writer, req.http)
				return handlerErr
			}
		})
	}
}

func (app *App) Handle(method, pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	app.Router.handle(method, pattern, handler, middlewares...)
}

func (app *App) HandleCtx(method, pattern string, handler Handler, middlewares ...MiddlewareFunc) {
	app.Router.handleCtx(method, pattern, handler, middlewares...)
}

func (app *App) HandleContext(method, pattern string, handler ContextHandlerFunc, middlewares ...MiddlewareFunc) {
	app.Router.handleContext(method, pattern, handler, middlewares...)
}

func (app *App) Raw(method, pattern string, handler RawHandlerFunc) {
	app.Router.handleRaw(method, pattern, handler)
}

func (app *App) GET(pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	app.Handle(http.MethodGet, pattern, handler, middlewares...)
}

func (app *App) Get(pattern string, handler Handler, middlewares ...MiddlewareFunc) {
	app.HandleCtx(http.MethodGet, pattern, handler, middlewares...)
}

func (app *App) GETContext(pattern string, handler ContextHandlerFunc, middlewares ...MiddlewareFunc) {
	app.HandleContext(http.MethodGet, pattern, handler, middlewares...)
}

func (app *App) POST(pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	app.Handle(http.MethodPost, pattern, handler, middlewares...)
}

func (app *App) Post(pattern string, handler Handler, middlewares ...MiddlewareFunc) {
	app.HandleCtx(http.MethodPost, pattern, handler, middlewares...)
}

func (app *App) POSTContext(pattern string, handler ContextHandlerFunc, middlewares ...MiddlewareFunc) {
	app.HandleContext(http.MethodPost, pattern, handler, middlewares...)
}

func (app *App) PUT(pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	app.Handle(http.MethodPut, pattern, handler, middlewares...)
}

func (app *App) Put(pattern string, handler Handler, middlewares ...MiddlewareFunc) {
	app.HandleCtx(http.MethodPut, pattern, handler, middlewares...)
}

func (app *App) PUTContext(pattern string, handler ContextHandlerFunc, middlewares ...MiddlewareFunc) {
	app.HandleContext(http.MethodPut, pattern, handler, middlewares...)
}

func (app *App) PATCH(pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	app.Handle(http.MethodPatch, pattern, handler, middlewares...)
}

func (app *App) Patch(pattern string, handler Handler, middlewares ...MiddlewareFunc) {
	app.HandleCtx(http.MethodPatch, pattern, handler, middlewares...)
}

func (app *App) PATCHContext(pattern string, handler ContextHandlerFunc, middlewares ...MiddlewareFunc) {
	app.HandleContext(http.MethodPatch, pattern, handler, middlewares...)
}

func (app *App) DELETE(pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	app.Handle(http.MethodDelete, pattern, handler, middlewares...)
}

func (app *App) Delete(pattern string, handler Handler, middlewares ...MiddlewareFunc) {
	app.HandleCtx(http.MethodDelete, pattern, handler, middlewares...)
}

func (app *App) DELETEContext(pattern string, handler ContextHandlerFunc, middlewares ...MiddlewareFunc) {
	app.HandleContext(http.MethodDelete, pattern, handler, middlewares...)
}

func (app *App) Server(addr string) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           app,
		ReadHeaderTimeout: app.Config.ReadHeaderTimeout,
		ReadTimeout:       app.Config.ReadTimeout,
		WriteTimeout:      app.Config.WriteTimeout,
		IdleTimeout:       app.Config.IdleTimeout,
		MaxHeaderBytes:    app.Config.MaxHeaderBytes,
	}
}

func (app *App) Shutdown(ctx context.Context, server *http.Server) error {
	return server.Shutdown(ctx)
}

func FromHTTPHandler(handler http.Handler) HandlerFunc {
	return func(req *Request, res *Response) error {
		handler.ServeHTTP(res.writer, req.http)
		return nil
	}
}

func FromHTTPHandlerFunc(handler http.HandlerFunc) HandlerFunc {
	return FromHTTPHandler(handler)
}
