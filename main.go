package main

import (
	"embed"
	"os"

	"github.com/MISSmihu/MHcode/internal/appupdate"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed all:skills
var bundledSkills embed.FS

func main() {
	if handled, exitCode := appupdate.HandleCommandLine(os.Args[1:]); handled {
		os.Exit(exitCode)
	}
	appupdate.ScheduleCleanup(os.Args[1:])
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "MHcode",
		Width:  1280,
		Height: 820,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 247, G: 248, B: 245, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
