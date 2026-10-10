package main

import (
	"embed"
	"log"

	"github.com/ryanzhangtianran/Clash-One/internal/app"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/tray.png
var trayIcon []byte

func main() {
	if err := app.Run(assets, trayIcon); err != nil {
		log.Fatal(err)
	}
}
