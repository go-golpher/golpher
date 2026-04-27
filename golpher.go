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
}

func (app *App) UseHTTP(middlewares ...func(http.Handler) http.Handler) {
	for _, middleware := range middlewares {
		mw := middleware
		app.Use(func(next HandlerFunc) HandlerFunc {
			return func(req *Request, res *Response) error {
				var handlerErr error
				h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					request := &Request{http: r, params: req.params}
					response := &Response{writer: w}
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

func (app *App) GET(pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	app.Handle(http.MethodGet, pattern, handler, middlewares...)
}

func (app *App) POST(pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	app.Handle(http.MethodPost, pattern, handler, middlewares...)
}

func (app *App) PUT(pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	app.Handle(http.MethodPut, pattern, handler, middlewares...)
}

func (app *App) PATCH(pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	app.Handle(http.MethodPatch, pattern, handler, middlewares...)
}

func (app *App) DELETE(pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	app.Handle(http.MethodDelete, pattern, handler, middlewares...)
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
