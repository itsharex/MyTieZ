package app

import (
	"fmt"
	"math"
	"strings"

	"mytiez/internal/store"

	"github.com/egoist/mygo/ui"
)

type accentPickerState struct {
	h, s, v  float64
	lastHex  string
	hexInput string
}

// renderCompactAccentPicker 渲染紧凑的左右三栏取色盘（方盘 + 竖直色相条 + 预览与 Hex 输入）。
func (a *clipApp) renderCompactAccentPicker(c *ui.Context) {
	t := c.Theme()
	cur := ui.RGB(t.Accent.R, t.Accent.G, t.Accent.B)
	curHex := fmt.Sprintf("#%02x%02x%02x", cur.R, cur.G, cur.B)

	if a.accentPicker == nil {
		h, s, v := rgbToHSV(cur.R, cur.G, cur.B)
		a.accentPicker = &accentPickerState{
			h:        h,
			s:        s,
			v:        v,
			lastHex:  curHex,
			hexInput: strings.ToUpper(curHex),
		}
	}
	st := a.accentPicker
	if curHex != st.lastHex {
		h, s, v := rgbToHSV(cur.R, cur.G, cur.B)
		// 纯黑白或灰色时保留原有色相，避免拖动到边缘时色相条跳回红色。
		if s > 0.001 {
			st.h = h
		}
		st.s, st.v = s, v
		st.lastHex = curHex
		st.hexInput = strings.ToUpper(curHex)
	}

	applyHSV := func(h, s, v float64) {
		st.h = math.Mod(math.Max(0, h), 360)
		st.s = clamp01(s)
		st.v = clamp01(v)
		col := hsvToRGB(st.h, st.s, st.v)
		hex := fmt.Sprintf("#%02x%02x%02x", col.R, col.G, col.B)
		st.lastHex = hex
		st.hexInput = strings.ToUpper(hex)
		a.settings.AccentColor = hex
		a.schedulePersistSettings()
	}

	const (
		panelH    float32 = 96
		hueW      float32 = 12
		infoW     float32 = 100
		padRadius float32 = 8
	)

	ui.Box(c).FillWidth().Padding(10).Radius(radiusGroup).
		Background(t.Surface).Border(1, t.Border.Alpha(0.65)).
		Children(func() {
			ui.Row(c).FillWidth().Height(panelH).Gap(10).AlignItems(ui.Stretch).Children(func() {
				// 1. 左侧：饱和度/亮度取色方盘（自适应剩余宽度）
				svBox := ui.Box(c.Key("accent-sv-pad")).Grow(1).Height(panelH).
					Radius(padRadius).Clip().Cursor(ui.CursorCrosshair)
				svW := svBox.Bounds().W
				if svW <= 0 {
					svW = 176
				}
				svBox.HandleInput(func(ev ui.InputEvent) bool {
					switch ev.Kind {
					case ui.InputPointerDown:
						if ev.Button == 0 {
							applyHSV(st.h, float64(ev.X/svW), 1-float64(ev.Y/panelH))
							return true
						}
					case ui.InputPointerMove:
						if ev.Pressure > 0 {
							applyHSV(st.h, float64(ev.X/svW), 1-float64(ev.Y/panelH))
							return true
						}
					}
					return false
				})
				pureHue := hsvToRGB(st.h, 1, 1)
				svBox.Draw(func(p *ui.Painter, r ui.Rect) {
					p.FillGradient(r, ui.LinearGradient{From: ui.RGB(255, 255, 255), To: pureHue, Angle: 90}, padRadius)
					p.FillGradient(r, ui.LinearGradient{From: ui.RGBA(0, 0, 0, 0), To: ui.RGB(0, 0, 0), Angle: 180}, padRadius)
					cx := r.X + r.W*float32(st.s)
					cy := r.Y + r.H*float32(1-st.v)
					const d float32 = 12
					p.Stroke(ui.Rect{X: cx - d/2 - 1, Y: cy - d/2 - 1, W: d + 2, H: d + 2}, ui.RGBA(0, 0, 0, 0.35), (d+2)/2, 1)
					p.Stroke(ui.Rect{X: cx - d/2, Y: cy - d/2, W: d, H: d}, ui.RGB(255, 255, 255), d/2, 2)
				})

				// 2. 中间：竖直圆角色相条
				hueBar := ui.Box(c.Key("accent-hue-bar")).Size(hueW, panelH).Shrink(0).
					Radius(hueW / 2).Cursor(ui.CursorPointer)
				hueBar.HandleInput(func(ev ui.InputEvent) bool {
					switch ev.Kind {
					case ui.InputPointerDown:
						if ev.Button == 0 {
							applyHSV(clamp01(float64(ev.Y/panelH))*360, st.s, st.v)
							return true
						}
					case ui.InputPointerMove:
						if ev.Pressure > 0 {
							applyHSV(clamp01(float64(ev.Y/panelH))*360, st.s, st.v)
							return true
						}
					}
					return false
				})
				hueBar.Draw(func(p *ui.Painter, r ui.Rect) {
					hues := []ui.Color{
						ui.RGB(255, 0, 0),
						ui.RGB(255, 255, 0),
						ui.RGB(0, 255, 0),
						ui.RGB(0, 255, 255),
						ui.RGB(0, 0, 255),
						ui.RGB(255, 0, 255),
						ui.RGB(255, 0, 0),
					}
					capR := r.W / 2
					p.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: r.W}, hues[0], capR)
					p.Fill(ui.Rect{X: r.X, Y: r.Y + r.H - r.W, W: r.W, H: r.W}, hues[6], capR)
					innerH := r.H - r.W
					segH := innerH / 6
					for i := range 6 {
						p.FillGradient(
							ui.Rect{X: r.X, Y: r.Y + capR + segH*float32(i), W: r.W, H: segH + 0.5},
							ui.LinearGradient{From: hues[i], To: hues[i+1], Angle: 180},
							0,
						)
					}
					ky := r.Y + r.H*float32(st.h/360)
					const kh float32 = 6
					ky = max(r.Y+kh/2+2, min(r.Y+r.H-kh/2-2, ky))
					knob := ui.Rect{X: r.X + 1.5, Y: ky - kh/2, W: r.W - 3, H: kh}
					p.Fill(knob, ui.RGB(255, 255, 255), kh/2)
					p.Stroke(knob, ui.RGBA(0, 0, 0, 0.28), kh/2, 1)
				})

				// 3. 右侧：微圆角 HEX 输入 + 紧凑 RGB 数值
				ui.Column(c).Width(infoW).Height(panelH).Shrink(0).Justify(ui.Center).Gap(8).Children(func() {
					ui.Column(c).FillWidth().Gap(4).Children(func() {
						ui.Text(c, "HEX").FontSize(10.5).FontWeight(500).TextColor(t.TextMuted).SingleLine()
						hexBox := ui.TextInputBase(c.Key("accent-hex-input"), &st.hexInput).
							FillWidth().Height(28).Padding(0, 8).Radius(radiusMicro).
							FontSize(12).FontFeatures("tnum").TextColor(t.Text).
							Background(t.SurfaceHover).Border(1, t.Border.Alpha(0.75)).
							Label("Hex")
						if hexBox.Focused() {
							hexBox.Border(1.5, t.Accent)
						}
						if hexBox.Changed() {
							raw := strings.TrimSpace(st.hexInput)
							if len(raw) == 6 && raw[0] != '#' {
								raw = "#" + raw
							}
							if norm := store.NormalizeHexColor(raw); norm != "" {
								col := ui.Hex(norm)
								h, s, v := rgbToHSV(col.R, col.G, col.B)
								st.h, st.s, st.v = h, s, v
								st.lastHex = norm
								a.settings.AccentColor = norm
								a.schedulePersistSettings()
							}
						}
					})

					rgbText := fmt.Sprintf("R %d  G %d  B %d", cur.R, cur.G, cur.B)
					ui.Text(c, rgbText).FontSize(10.5).FontFeatures("tnum").TextColor(t.TextMuted).SingleLine()
				})
			})
		})
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func rgbToHSV(r, g, b uint8) (h, s, v float64) {
	rf := float64(r) / 255
	gf := float64(g) / 255
	bf := float64(b) / 255
	maxC := max(rf, max(gf, bf))
	minC := min(rf, min(gf, bf))
	delta := maxC - minC
	v = maxC
	if maxC > 0 {
		s = delta / maxC
	}
	if delta == 0 {
		return 0, s, v
	}
	switch maxC {
	case rf:
		h = 60 * math.Mod((gf-bf)/delta, 6)
	case gf:
		h = 60 * ((bf-rf)/delta + 2)
	default:
		h = 60 * ((rf-gf)/delta + 4)
	}
	if h < 0 {
		h += 360
	}
	return h, s, v
}

func hsvToRGB(h, s, v float64) ui.Color {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := v - c
	var r1, g1, b1 float64
	switch {
	case h < 60:
		r1, g1, b1 = c, x, 0
	case h < 120:
		r1, g1, b1 = x, c, 0
	case h < 180:
		r1, g1, b1 = 0, c, x
	case h < 240:
		r1, g1, b1 = 0, x, c
	case h < 300:
		r1, g1, b1 = x, 0, c
	default:
		r1, g1, b1 = c, 0, x
	}
	return ui.RGB(
		uint8(math.Round((r1+m)*255)),
		uint8(math.Round((g1+m)*255)),
		uint8(math.Round((b1+m)*255)),
	)
}
