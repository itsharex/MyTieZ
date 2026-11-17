package main

import (
	_ "embed"

	"mytiez/internal/app"
)

//go:embed resources/icon.png
var appIconPNG []byte

func main() {
	app.Run(appIconPNG)
}
