package main

import (
	"embed"
	"flag"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Флаги для смоук-теста; в настройки не сохраняются.
	game := flag.String("game", "", "папка игры (где лежит Logs)")
	shotsDir := flag.String("shots", "", "папка скриншотов")
	flag.Parse()

	app := NewApp(*game, *shotsDir)

	err := wails.Run(&options.App{
		Title:            "Tarkov Map",
		Width:            1280,
		Height:           800,
		BackgroundColour: &options.RGBA{R: 20, G: 19, B: 15, A: 1},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		Bind:             []interface{}{app},
	})
	if err != nil {
		log.Fatal(err)
	}
}
