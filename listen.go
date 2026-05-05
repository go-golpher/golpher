package golpher

import (
	"fmt"
	"log"
	"net"
	"net/http"
)

type ListenConfig struct {
	Silent bool
}

var figletGolpher = `
 ______     ______     __         ______   __  __     ______     ______    
/\  ___\   /\  __ \   /\ \       /\  == \ /\ \_\ \   /\  ___\   /\  == \   
\ \ \__ \  \ \ \/\ \  \ \ \____  \ \  _-/ \ \  __ \  \ \  __\   \ \  __<   
 \ \_____\  \ \_____\  \ \_____\  \ \_\    \ \_\ \_\  \ \_____\  \ \_\ \_\ 	%s
  \/_____/   \/_____/   \/_____/   \/_/     \/_/\/_/   \/_____/   \/_/ /_/	Listen in port: %v`

func (app *App) Listen(configs ...ListenConfig) {
	var cfg ListenConfig
	if len(configs) > 0 {
		cfg = configs[0]
	}
	port := fmt.Sprintf(":%v", app.Config.Port)
	if !cfg.Silent && !app.Config.DisableBanner {
		log.Printf(figletGolpher, version, port)
	}
	if err := app.Server(port).ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func (app *App) Serve(listener net.Listener) error {
	return app.Server("").Serve(listener)
}
