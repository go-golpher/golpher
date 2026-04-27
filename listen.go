package golpher

import (
	"fmt"
	"log"
	"net"
	"net/http"
)

type ListenConfig struct {
}

var figletGolpher = `
 ______     ______     __         ______   __  __     ______     ______    
/\  ___\   /\  __ \   /\ \       /\  == \ /\ \_\ \   /\  ___\   /\  == \   
\ \ \__ \  \ \ \/\ \  \ \ \____  \ \  _-/ \ \  __ \  \ \  __\   \ \  __<   
 \ \_____\  \ \_____\  \ \_____\  \ \_\    \ \_\ \_\  \ \_____\  \ \_\ \_\ 	%s
  \/_____/   \/_____/   \/_____/   \/_/     \/_/\/_/   \/_____/   \/_/ /_/	Listen in port: %v`

func (app *App) Listen(configs ...ListenConfig) {
	port := fmt.Sprintf(":%v", app.Config.Port)
	listener, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("Error binding to %s: %v", port, err)
		return
	}
	log.Println(fmt.Sprintf(figletGolpher, version, port))
	if err := http.Serve(listener, app); err == nil {
		log.Fatal(err)
	}
}
