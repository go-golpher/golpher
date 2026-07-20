package golpher

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// MethodQuery is the HTTP QUERY method as defined by RFC 10008.
// QUERY is safe and idempotent, like GET, but carries a request body.
const MethodQuery = "QUERY"

// AppConfig is the public configuration passed to New. After construction
// the app holds a private snapshot; mutating a local AppConfig afterward
// has no effect on the running app.
type AppConfig struct {
	ErrorHandler  ErrorHandlerFunc
	ErrorObserver ErrorObserverFunc
	Port          int

	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxHeaderBytes    int
	DisableBanner     bool

	// EnableResponseBodyCapture opts into capturing the response body for
	// inspection via Response.Body() / BodyString(). Default false.
	EnableResponseBodyCapture bool

	// MaxRequestBodyBytes enforces a body size limit via lazy
	// http.MaxBytesReader.  0 = 1 MiB default, negative = unlimited,
	// positive = enforce that many bytes.
	MaxRequestBodyBytes int64
}

type appConfig struct {
	Port                int
	ReadHeaderTimeout   time.Duration
	ReadTimeout         time.Duration
	WriteTimeout        time.Duration
	IdleTimeout         time.Duration
	MaxHeaderBytes      int
	DisableBanner       bool
	EnableBodyCapture   bool
	MaxRequestBodyBytes int64 // effective limit (0 default resolved to 1<<20)
}

// App is a Golpher application. It implements http.Handler and can be
// deployed directly with net/http. Configuration is supplied through
// AppConfig at construction time and becomes immutable after the first
// request is served.
type App struct {
	config        appConfig
	router        *Router
	errorHandler  ErrorHandlerFunc
	errorObserver ErrorObserverFunc
	middlewares   []MiddlewareFunc

	lifecycleMu sync.Mutex
	frozen      atomic.Bool
}

// New creates an App from the supplied configuration. When no config is
// passed, sensible defaults are used (port 8080, timeouts, 1 MiB body
// limit). The app copies the caller's config so later mutation has no
// effect.
func New(configs ...AppConfig) *App {
	var cfg AppConfig
	if len(configs) > 0 {
		cfg = configs[0]
	}

	bodyLimit := cfg.MaxRequestBodyBytes
	if bodyLimit == 0 {
		bodyLimit = 1 << 20 // 1 MiB
	}

	rh := cfg.ReadHeaderTimeout
	if rh == 0 {
		rh = 5 * time.Second
	}
	rt := cfg.ReadTimeout
	if rt == 0 {
		rt = 10 * time.Second
	}
	wt := cfg.WriteTimeout
	if wt == 0 {
		wt = 30 * time.Second
	}
	it := cfg.IdleTimeout
	if it == 0 {
		it = 120 * time.Second
	}

	port := cfg.Port
	if port == 0 {
		port = 8080
	}

	eh := cfg.ErrorHandler
	if eh == nil {
		eh = defaultErrorHandler
	}

	app := &App{
		config: appConfig{
			Port:                port,
			ReadHeaderTimeout:   rh,
			ReadTimeout:         rt,
			WriteTimeout:        wt,
			IdleTimeout:         it,
			MaxHeaderBytes:      cfg.MaxHeaderBytes,
			DisableBanner:       cfg.DisableBanner,
			EnableBodyCapture:   cfg.EnableResponseBodyCapture,
			MaxRequestBodyBytes: bodyLimit,
		},
		errorHandler:  eh,
		errorObserver: cfg.ErrorObserver,
	}
	app.router = &Router{app: app}
	return app
}

// ServeHTTP implements http.Handler. The first call freezes the route
// table; subsequent route/middleware registrations panic.
func (app *App) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	app.freezeOnce()
	app.router.ServeHTTP(w, req)
}

// freezeOnce atomically freezes the app if not already frozen.
func (app *App) freezeOnce() {
	if app.frozen.Load() {
		return
	}
	app.lifecycleMu.Lock()
	app.frozen.Swap(true)
	app.lifecycleMu.Unlock()
}

// IsFrozen reports whether the app has served its first request.
func (app *App) IsFrozen() bool {
	return app.frozen.Load()
}

// checkFrozen panics if the app has been frozen by a prior ServeHTTP call.
func (app *App) checkFrozen() {
	if app.frozen.Load() {
		panic("golpher: app is frozen after ServeHTTP")
	}
}

// checkFrozenLocked must be called while holding lifecycleMu.
func (app *App) checkFrozenLocked() {
	if app.frozen.Load() {
		panic("golpher: app is frozen after ServeHTTP")
	}
}

// Handle registers a route for the given method and pattern. Verb
// shorthands (GET, POST, …) delegate here.
func (app *App) Handle(method, pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	app.checkFrozen()
	app.lifecycleMu.Lock()
	defer app.lifecycleMu.Unlock()
	app.checkFrozenLocked()
	app.router.handle(method, pattern, handler, middlewares...)
}

// Use appends global middleware. It panics if the app has served a request.
func (app *App) Use(middlewares ...MiddlewareFunc) {
	app.checkFrozen()
	app.lifecycleMu.Lock()
	defer app.lifecycleMu.Unlock()
	app.checkFrozenLocked()
	app.middlewares = append(app.middlewares, middlewares...)
	app.router.rebuildHandlers()
}

// UseHTTP adapts standard-library func(http.Handler) http.Handler
// middleware into the middleware chain. The adapted handler writes
// through the tracking Response writer so status, bytes, and capture
// are counted. Errors are routed to the observer and error handler.
//
// If the standard middleware writes a response before calling next,
// the response is committed and the error renderer cannot append a
// second error body. Use ErrorObserver for programmatic observation
// of errors after commit.
func (app *App) UseHTTP(middlewares ...func(http.Handler) http.Handler) {
	for _, mw := range middlewares {
		middleware := mw
		app.Use(func(next HandlerFunc) HandlerFunc {
			return func(req *Request, res *Response) error {
				var handlerErr error
				h := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					handlerErr = next(req, res)
				}))
				h.ServeHTTP(res.Raw(), req.http)
				if handlerErr != nil {
					app.reportError(req, res, handlerErr)
				}
				return nil
			}
		})
	}
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

// QUERY registers a handler for the HTTP QUERY method (RFC 10008).
func (app *App) QUERY(pattern string, handler HandlerFunc, middlewares ...MiddlewareFunc) {
	app.Handle(MethodQuery, pattern, handler, middlewares...)
}

// Server returns a configured *http.Server for the given address.
func (app *App) Server(addr string) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           app,
		ReadHeaderTimeout: app.config.ReadHeaderTimeout,
		ReadTimeout:       app.config.ReadTimeout,
		WriteTimeout:      app.config.WriteTimeout,
		IdleTimeout:       app.config.IdleTimeout,
		MaxHeaderBytes:    app.config.MaxHeaderBytes,
	}
}

// Shutdown gracefully shuts down the server.
func (app *App) Shutdown(ctx context.Context, server *http.Server) error {
	return server.Shutdown(ctx)
}
