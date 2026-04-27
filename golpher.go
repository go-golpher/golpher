package golpher

import (
	"net/http"
)

type App struct {
	ErrorHandler ErrorHandlerFunc
	Router       *Router
	Config       AppConfig
}

type AppConfig struct {
	ErrorHandler ErrorHandlerFunc
	Port         int
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
	app := &App{
		ErrorHandler: cfg.ErrorHandler,
		Config:       cfg,
	}
	app.Router = &Router{
		mux: http.NewServeMux(),
		app: app,
	}
	return app
}

func (app *App) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	app.Router.mux.ServeHTTP(w, req)
}
