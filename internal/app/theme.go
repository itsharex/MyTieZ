package app

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	panelWidth     = 400
	panelHeight    = 660
	minPanelWidth  = 360
	minPanelHeight = 420

	radiusCard    float32 = 18
	radiusControl float32 = 14
	radiusGroup   float32 = 12
	radiusMicro   float32 = 6

	headerBtnSize    float32 = 32
	headerBtnRadius  float32 = 10
	searchBoxHeight  float32 = 46
	filterPillHeight float32 = 28
	pillHeight       float32 = 30
	cardActionSize   float32 = 22
	settingGroupMinH float32 = 52
)

var (
	myTiezLightTheme = buildMyTiezTheme(false)
	myTiezDarkTheme  = buildMyTiezTheme(true)

	colorTransition = ui.ElementTransition{
		Colors:   true,
		Duration: 140 * time.Millisecond,
	}
	cardTransition = ui.ElementTransition{
		Duration: 260 * time.Millisecond,
		Ease:     ui.EaseOut,
		Colors:   true,
		Shadows:  true,
	}
)

func buildMyTiezTheme(dark bool) *ui.Theme {
	if dark {
		t := ui.DarkTheme()
		t.Background = ui.Hex("#181818")
		t.Surface = ui.Hex("#242424")
		t.SurfaceHover = ui.Hex("#2e2e2e")
		t.SurfacePressed = ui.Hex("#383838")
		t.Border = ui.Hex("#363636")
		t.Text = ui.Hex("#e8e8e8")
		t.TextMuted = ui.Hex("#9e9e9e")
		t.Accent = ui.Hex("#3b82f6")
		t.AccentHover = ui.Hex("#60a5fa")
		t.AccentPressed = ui.Hex("#2563eb")
		t.AccentText = ui.Hex("#ffffff")
		t.InactiveAccent = t.Accent
		t.Danger = ui.Hex("#ef4444")
		t.Radius = radiusControl
		t.Spacing = 4.8
		t.FontSize = 13.5
		t.Font = "Microsoft YaHei UI"
		return t
	}
	t := ui.LightTheme()
	t.Background = ui.Hex("#f4f6f9")
	t.Surface = ui.Hex("#ffffff")
	t.SurfaceHover = ui.Hex("#f8fafc")
	t.SurfacePressed = ui.Hex("#eef2f6")
	t.Border = ui.Hex("#e4e7ec")
	t.Text = ui.Hex("#182235")
	t.TextMuted = ui.Hex("#646d7d")
	t.Accent = ui.Hex("#2f6fed")
	t.AccentHover = ui.Hex("#245dce")
	t.AccentPressed = ui.Hex("#1d4ed8")
	t.AccentText = ui.Hex("#ffffff")
	t.InactiveAccent = t.Accent
	t.Danger = ui.Hex("#c42b1c")
	t.Radius = radiusControl
	t.Spacing = 4.8
	t.FontSize = 13.5
	t.Font = "Microsoft YaHei UI"
	return t
}

func backdropVibrancy(mode string) mygo.Vibrancy {
	switch mode {
	case "acrylic":
		return mygo.VibrancyAcrylic
	case "solid":
		return mygo.VibrancyNone
	default:
		return mygo.VibrancyMica
	}
}

func (a *clipApp) usingVibrancy(c *ui.Context) bool {
	return c.Vibrancy() && a.settings.BackdropMode != "solid"
}

func (a *clipApp) applyBackdrop() {
	if a.panel == nil {
		return
	}
	a.panel.SetVibrancy(backdropVibrancy(a.settings.BackdropMode))
}

func (a *clipApp) resolveTheme(c *ui.Context) *ui.Theme {
	useDark := c.Theme().Dark
	switch a.settings.ColorMode {
	case "dark":
		useDark = true
	case "light":
		useDark = false
	}
	base := myTiezLightTheme
	if useDark {
		base = myTiezDarkTheme
	}
	vibrancy := a.usingVibrancy(c)
	scale := c.Preferences().TextScale
	if !vibrancy && (scale <= 0 || scale == 1) {
		return base
	}
	cloned := *base
	if scale > 0 && scale != 1 {
		cloned.FontSize *= scale
		cloned.Spacing *= scale
	}
	if vibrancy {
		if useDark {
			cloned.Surface = cloned.Surface.Alpha(0.72)
			cloned.SurfaceHover = cloned.SurfaceHover.Alpha(0.84)
		} else {
			cloned.Surface = cloned.Surface.Alpha(0.78)
			cloned.SurfaceHover = cloned.SurfaceHover.Alpha(0.9)
		}
		cloned.Border = cloned.Border.Alpha(0.55)
	}
	return &cloned
}
