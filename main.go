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

// Снимок данных tarkov.dev (scripts/seed): с ним exe работает без сети с первого запуска.
//
//go:embed assets/data.zip
var seedZip []byte

func main() {
	// Флаги для смоук-теста; в настройки не сохраняются.
	game := flag.String("game", "", "папка игры (где лежит Logs)")
	shotsDir := flag.String("shots", "", "папка скриншотов")
	flag.Parse()

	app := NewApp(*game, *shotsDir)

	width, height := 1280, 800
	if w := app.GetSettings().Window; w != nil && w.W > 0 && w.H > 0 {
		width, height = w.W, w.H
	}

	err := wails.Run(&options.App{
		Title:            "Tarkov Map",
		Width:            width,
		Height:           height,
		StartHidden:      true, // показывает domReady, уже на сохранённом месте
		BackgroundColour: &options.RGBA{R: 20, G: 19, B: 15, A: 1},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		OnBeforeClose:    app.beforeClose,
		Bind:             []interface{}{app},
	})
	if err != nil {
		log.Fatal(err)
	}
}
