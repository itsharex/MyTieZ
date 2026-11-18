package main

import (
	_ "embed"

	"mytiez/internal/app"
)

//go:embed resources/tray.png
var trayIconPNG []byte

func main() {
	app.Run(trayIconPNG)
}
