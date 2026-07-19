package golpher

import (
	"fmt"
	"log"
	"net"
	"net/http"
)

// ListenConfig controls the behaviour of App.Listen.
type ListenConfig struct {
	Silent bool
}

var figletGolpher = `
  ______     ______     __         ______   __  __     ______     ______
 /\  ___\   /\  __ \   /\ \       /\  == \ /\ \_\ \   /\  ___\   /\  == \
 \ \ \__ \  \ \ \/\ \  \ \ \____  \ \  _-/ \ \  __ \  \ \  __\   \ \  __<
  \ \_____\  \ \_____\  \ \_____\  \ \_\    \ \_\ \_\  \ \_____\  \ \_\ \_\ 	%s
   \/_____/   \/_____/   \/_____/   \/_/     \/_/\/_/   \/_____/   \/_/ /_/	Listen in port: %v`

// Listen starts the HTTP server on the configured port. On error it
// returns the result of ListenAndServe directly; the caller must decide
// how to handle startup failures. http.ErrServerClosed is normalised to
// nil so graceful shutdowns are not reported as errors.
func (app *App) Listen(configs ...ListenConfig) error {
	var cfg ListenConfig
	if len(configs) > 0 {
		cfg = configs[0]
	}
	port := fmt.Sprintf(":%v", app.config.Port)
	if !cfg.Silent && !app.config.DisableBanner {
		log.Printf(figletGolpher, version, port)
	}
	err := app.Server(port).ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Serve runs the application on the provided listener.
func (app *App) Serve(listener net.Listener) error {
	return app.Server("").Serve(listener)
}
