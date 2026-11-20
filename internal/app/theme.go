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
	radiusPill    float32 = 9999

	headerBtnSize    float32 = 32
	headerBtnRadius  float32 = 10
	searchBoxHeight  float32 = 44
	filterPillHeight float32 = 28
	pillHeight       float32 = 30
	cardActionSize   float32 = 22
)

func shadowAlpha(t *ui.Theme, light, dark float32) float32 {
	if t.Dark {
		return dark
	}
	return light
}

func surfaceBg(t *ui.Theme, hovered bool) ui.Color {
	if hovered {
		return t.SurfaceHover
	}
	return t.Surface
}

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
	settingsEnterTransition = ui.ElementTransition{
		Animation: &ui.Animation{
			Spring: ui.SpringOptions{
				Duration: 360 * time.Millisecond,
				Bounce:   0.25,
			},
		},
		Enter: &ui.Motion{X: 36, Opacity: 0},
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

type backdropPreset struct {
	vibrancy          mygo.Vibrancy
	rootOverlayDark   float32
	rootOverlayLight  float32
	surfaceDark       float32
	surfaceLight      float32
	surfaceHoverDark  float32
	surfaceHoverLight float32
	surfacePressDark  float32
	surfacePressLight float32
	borderDark        float32
	borderLight       float32
}

// Presets tuned after DeskBox's glass surfaces: keep DWM blur luminous and
// use subtle white-tinted glass cards with crisp hairline borders in dark mode.
var backdropPresets = map[string]backdropPreset{
	"mica": {
		vibrancy:          mygo.VibrancyMica,
		rootOverlayDark:   0,
		rootOverlayLight:  0,
		surfaceDark:       0.11,
		surfaceLight:      0.62,
		surfaceHoverDark:  0.17,
		surfaceHoverLight: 0.78,
		surfacePressDark:  0.22,
		surfacePressLight: 0.88,
		borderDark:        0.15,
		borderLight:       0.55,
	},
	"mica_alt": {
		vibrancy:          mygo.VibrancyTabbed,
		rootOverlayDark:   0.10,
		rootOverlayLight:  0.10,
		surfaceDark:       0.10,
		surfaceLight:      0.48,
		surfaceHoverDark:  0.16,
		surfaceHoverLight: 0.64,
		surfacePressDark:  0.21,
		surfacePressLight: 0.76,
		borderDark:        0.16,
		borderLight:       0.55,
	},
	"acrylic_base": {
		vibrancy:          mygo.VibrancyAcrylic,
		rootOverlayDark:   0.24,
		rootOverlayLight:  0.22,
		surfaceDark:       0.14,
		surfaceLight:      0.56,
		surfaceHoverDark:  0.20,
		surfaceHoverLight: 0.72,
		surfacePressDark:  0.25,
		surfacePressLight: 0.82,
		borderDark:        0.19,
		borderLight:       0.60,
	},
	"acrylic": {
		vibrancy:          mygo.VibrancyAcrylic,
		rootOverlayDark:   0.06,
		rootOverlayLight:  0,
		surfaceDark:       0.11,
		surfaceLight:      0.38,
		surfaceHoverDark:  0.17,
		surfaceHoverLight: 0.54,
		surfacePressDark:  0.22,
		surfacePressLight: 0.68,
		borderDark:        0.16,
		borderLight:       0.55,
	},
}

func backdropPresetFor(mode string) (backdropPreset, bool) {
	if mode == "solid" {
		return backdropPreset{}, false
	}
	if p, ok := backdropPresets[mode]; ok {
		return p, true
	}
	return backdropPresets["mica"], true
}

func backdropVibrancy(mode string) mygo.Vibrancy {
	if p, ok := backdropPresetFor(mode); ok {
		return p.vibrancy
	}
	return mygo.VibrancyNone
}

func (a *clipApp) usingVibrancy(c *ui.Context) bool {
	return c.Vibrancy() && a.settings.BackdropMode != "solid"
}

func (a *clipApp) rootBackground(c *ui.Context, t *ui.Theme) ui.Color {
	if !a.usingVibrancy(c) {
		return t.Background
	}
	p, ok := backdropPresetFor(a.settings.BackdropMode)
	if !ok {
		return t.Background
	}
	if t.Dark {
		if p.rootOverlayDark <= 0 {
			return ui.Transparent
		}
		// Cool deep tint counteracts Win11 DWM acrylic's warm gray haze.
		return ui.RGBA(14, 17, 24, p.rootOverlayDark)
	}
	if p.rootOverlayLight <= 0 {
		return ui.Transparent
	}
	return t.Background.Alpha(p.rootOverlayLight)
}

func (a *clipApp) floatingSurface(c *ui.Context, t *ui.Theme) ui.Color {
	if !a.usingVibrancy(c) {
		return t.Surface
	}
	if t.Dark {
		return myTiezDarkTheme.Surface.Alpha(0.90)
	}
	return myTiezLightTheme.Surface.Alpha(0.94)
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
		p, _ := backdropPresetFor(a.settings.BackdropMode)
		if useDark {
			glass := ui.RGB(255, 255, 255)
			cloned.Surface = glass.Alpha(p.surfaceDark)
			cloned.SurfaceHover = glass.Alpha(p.surfaceHoverDark)
			cloned.SurfacePressed = glass.Alpha(p.surfacePressDark)
			cloned.Border = glass.Alpha(p.borderDark)
			cloned.Text = ui.RGB(245, 246, 248)
			cloned.TextMuted = glass.Alpha(0.76)
		} else {
			cloned.Surface = cloned.Surface.Alpha(p.surfaceLight)
			cloned.SurfaceHover = cloned.SurfaceHover.Alpha(p.surfaceHoverLight)
			cloned.SurfacePressed = cloned.SurfacePressed.Alpha(p.surfacePressLight)
			cloned.Border = cloned.Border.Alpha(p.borderLight)
			cloned.TextMuted = cloned.Text.Alpha(0.74)
		}
	}
	return &cloned
}
