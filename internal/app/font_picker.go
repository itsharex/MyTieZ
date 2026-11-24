package app

import (
	"slices"
	"strings"

	"mytiez/locales"

	"github.com/egoist/mygo/ui"
)

const defaultUIFont = "Microsoft YaHei UI"

var preferredFontOrder = []string{
	"HarmonyOS Sans SC",
	"PingFang SC",
	"MiSans",
	"Noto Sans SC",
	"Source Han Sans SC",
	"Alibaba PuHuiTi",
	"Sarasa Gothic SC",
	"Segoe UI Variable",
	"Segoe UI",
	"Microsoft YaHei UI",
	"Microsoft YaHei",
	"SimHei",
	"Consolas",
	"JetBrains Mono",
}

// ensureSystemFonts 仅在字体选择器首次展开时按需枚举并缓存系统字体，避免启动耗时。
func (a *clipApp) ensureSystemFonts() {
	if a.systemFontsLoaded {
		return
	}
	a.systemFontsLoaded = true
	raw := ui.FontFamilies()
	if len(raw) == 0 {
		return
	}

	rank := make(map[string]int, len(preferredFontOrder))
	for i, name := range preferredFontOrder {
		rank[strings.ToLower(name)] = i
	}

	filtered := make([]ui.FontFamily, 0, len(raw))
	for _, f := range raw {
		name := strings.TrimSpace(f.Name)
		if name == "" || strings.HasPrefix(name, "@") {
			continue
		}
		f.Name = name
		filtered = append(filtered, f)
	}

	slices.SortStableFunc(filtered, func(x, y ui.FontFamily) int {
		rx, okX := rank[strings.ToLower(x.Name)]
		ry, okY := rank[strings.ToLower(y.Name)]
		if okX && okY {
			return rx - ry
		}
		if okX {
			return -1
		}
		if okY {
			return 1
		}
		return strings.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name))
	})
	a.systemFonts = filtered
}

func (a *clipApp) setFontFamily(family string) {
	a.settings.FontFamily = strings.TrimSpace(family)
	a.persistSettings()
}

func (a *clipApp) renderFontPickerSection(c *ui.Context, msgs locales.Messages) {
	t := c.Theme()
	curFont := strings.TrimSpace(a.settings.FontFamily)
	displayLabel := curFont
	if displayLabel == "" {
		displayLabel = fmtDefaultFontLabel(msgs)
	}

	ui.Column(c).FillWidth().Padding(8, 16).Gap(8).Children(func() {
		// 触发行：左侧标题 + 右侧胶囊按钮
		ui.Row(c).FillWidth().MinHeight(36).AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(12).Children(func() {
			a.renderSettingTitle(c, msgs.SettingsFont(), "").Shrink(0)
			trig := ui.ButtonBase(c).Height(pillHeight).MaxWidth(210).Padding(0, 12).Radius(radiusGroup).
				Justify(ui.SpaceBetween).AlignItems(ui.Center).Gap(8).
				Label(msgs.SettingsFontPickerA11y())
			bg := t.Surface
			border := t.Border.Alpha(0.7)
			if a.expanded.fontPicker {
				bg = t.Accent.Alpha(0.10)
				border = t.Accent.Alpha(0.55)
			} else if trig.Hovered() {
				bg = t.SurfaceHover
				border = t.Border
			}
			trig.Background(bg).Border(1, border).Transition(colorTransition).
				OnClick(func() {
					a.confirmAction = ""
					a.expanded.fontPicker = !a.expanded.fontPicker
					if a.expanded.fontPicker {
						a.ensureSystemFonts()
					}
				}).
				Children(func() {
					ui.Text(c, displayLabel).FontSize(13).FontWeight(600).TextColor(t.Text).SingleLine().Shrink(1)
					rot := float32(0)
					if a.expanded.fontPicker {
						rot = 180
					}
					ui.Icon(c, iconChevronDown).FontSize(14).TextColor(t.TextMuted).Shrink(0).Rotate(rot)
				})
		})

		if !a.expanded.fontPicker {
			return
		}
		a.ensureSystemFonts()

		// 展开面板：搜索框 + 可滚动字体预览列表
		ui.Column(c).FillWidth().Padding(10).Gap(8).Radius(radiusGroup).
			Background(t.Surface).Border(1, t.Border.Alpha(0.65)).
			Children(func() {
				ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
					searchWrap := ui.Row(c).Grow(1).Height(30).Padding(0, 8).Gap(6).AlignItems(ui.Center).
						Radius(radiusMicro).Background(t.SurfaceHover).Border(1, t.Border.Alpha(0.65))
					searchWrap.Children(func() {
						ui.Icon(c, iconSearch).FontSize(13).TextColor(t.TextMuted).Shrink(0)
						inp := ui.TextInputBase(c.Key("font-search"), &a.fontQuery).
							Grow(1).FontSize(12.5).TextColor(t.Text).
							Placeholder(msgs.SettingsFontSearch()).Label(msgs.SettingsFontSearch())
						if inp.Focused() {
							searchWrap.Border(1.5, t.Accent)
						}
						if a.fontQuery != "" {
							clr := ui.ButtonBase(c).Size(20, 20).Radius(10).Center().Label(msgs.SearchClear())
							if clr.Hovered() {
								clr.Background(t.SurfacePressed)
							}
							clr.OnClick(func() {
								a.fontQuery = ""
							}).Children(func() {
								ui.Icon(c, iconClear).FontSize(11).TextColor(t.TextMuted)
							})
						}
					})

					if curFont != "" {
						resetBtn := ui.ButtonBase(c).Height(30).Padding(0, 8).Radius(radiusMicro).Center().
							Label(msgs.SettingsFontReset())
						bg := t.SurfaceHover
						if resetBtn.Hovered() {
							bg = t.SurfacePressed
						}
						resetBtn.Background(bg).Transition(colorTransition).
							OnClick(func() {
								a.confirmAction = ""
								a.setFontFamily("")
							}).
							Children(func() {
								ui.Text(c, msgs.SettingsFontReset()).FontSize(12).FontWeight(500).TextColor(t.TextMuted).SingleLine()
							})
					}
				})

				q := strings.ToLower(strings.TrimSpace(a.fontQuery))
				showDefault := q == "" ||
					strings.Contains(strings.ToLower(msgs.SettingsFontDefault()), q) ||
					strings.Contains(strings.ToLower(defaultUIFont), q)

				matches := make([]ui.FontFamily, 0, len(a.systemFonts))
				for _, f := range a.systemFonts {
					if q == "" || strings.Contains(strings.ToLower(f.Name), q) {
						matches = append(matches, f)
					}
				}

				ui.Scroll(c.Key("font-picker-scroll")).FillWidth().Height(184).Children(func() {
					ui.Column(c).FillWidth().Gap(2).Children(func() {
						if showDefault {
							a.renderFontOptionRow(c, "", fmtDefaultFontLabel(msgs), false, curFont == "")
						}
						for _, f := range matches {
							a.renderFontOptionRow(c, f.Name, f.Name, f.Monospaced, strings.EqualFold(curFont, f.Name))
						}
						if !showDefault && len(matches) == 0 {
							ui.Column(c).FillWidth().Height(120).Center().Children(func() {
								ui.Text(c, msgs.SettingsFontEmpty()).FontSize(12.5).TextColor(t.TextMuted)
							})
						}
					})
				})
			})
	})
}

func fmtDefaultFontLabel(msgs locales.Messages) string {
	return msgs.SettingsFontDefault() + " (" + defaultUIFont + ")"
}

func (a *clipApp) renderFontOptionRow(c *ui.Context, family, label string, mono, selected bool) {
	t := c.Theme()
	btn := ui.ButtonBase(c).FillWidth().Height(32).Padding(0, 10).Radius(radiusMicro).
		Justify(ui.SpaceBetween).AlignItems(ui.Center).Gap(8).Label(label)
	bg := ui.Transparent
	if selected {
		bg = t.Accent.Alpha(0.12)
	} else if btn.Hovered() {
		bg = t.SurfaceHover
	}
	fg := t.Text
	weight := 500
	if selected {
		fg = t.Accent
		weight = 600
	}
	btn.Background(bg).Transition(colorTransition).
		OnClick(func() {
			a.confirmAction = ""
			a.setFontFamily(family)
		}).
		Children(func() {
			txt := ui.Text(c, label).FontSize(13).FontWeight(weight).TextColor(fg).SingleLine().Shrink(1)
			if family != "" {
				txt.Font(family)
			}
			ui.Row(c).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
				if mono {
					ui.Text(c, "Mono").FontSize(10.5).TextColor(t.TextMuted).SingleLine()
				}
				if selected {
					ui.Box(c).Size(7, 7).Radius(3.5).Background(t.Accent)
				}
			})
		})
}
