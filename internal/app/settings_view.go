package app

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var availableHotkeys = []string{"Alt+V", "Ctrl+Shift+V", "Alt+Shift+V", "Ctrl+Alt+V"}

// Zero value keeps every group collapsed by default.
type settingsGroupExpanded struct {
	general    bool
	clipboard  bool
	appearance bool
	data       bool
}

func (a *clipApp) settingsView(c *ui.Context) {
	t := c.Theme()

	ui.Column(c.Key("settings-page")).Fill().Children(func() {
		// 设置页顶部导航栏（与主页顶栏同构：外层 Padding(10, 16) + 内层 Height(36)）
		ui.Column(c).FillWidth().Padding(10, 16).Children(func() {
			ui.Row(c).Height(headerBtnSize).AlignItems(ui.Center).Gap(8).DragWindow().Children(func() {
				a.renderHeaderIconBtn(c, iconBack, "返回", false, false, func() {
					a.showSettings = false
					a.confirmAction = ""
					a.settingsNotice = ""
				})

				ui.Text(c, "设置").FontSize(22).Bold().TextColor(t.Text)
				ui.Box(c).Grow(1)
				a.renderWindowActionButtons(c)
			})
		})

		ui.Divider(c)

		ui.Scroll(c.Key("settings-scroll")).Grow(1).Children(func() {
			ui.Column(c).FillWidth().Padding(12, 12, 16, 12).Gap(8).Children(func() {
				if a.settingsNotice != "" {
					ui.Text(c, a.settingsNotice).FontSize(13).TextColor(t.Accent).
						FillWidth().Padding(10, 14).Radius(radiusGroup).
						Background(t.Surface.Mix(t.Accent, 0.1)).
						Border(1, t.Accent.Alpha(0.4))
				}

				// 1. 常规设置
				a.renderSettingsGroup(c, "常规设置", &a.expanded.general, func() {
					a.renderSwitchRow(c, "开机自启动", "", &a.settings.AutoStart, func(on bool) {
						if err := mygo.App.SetOpenAtLogin(on); err != nil {
							log.Printf("set open at login: %v", err)
							a.settings.AutoStart = !on
							a.settingsNotice = "设置开机自启动失败: " + err.Error()
						}
						a.persistSettings()
					})
					a.renderSwitchRow(c, "静默启动", "", &a.settings.SilentStart, func(bool) {
						a.persistSettings()
					})
					a.renderSwitchRow(c, "跟随鼠标位置呼出", "", &a.settings.FollowMouse, func(bool) {
						a.persistSettings()
					})
					a.renderSwitchRow(c, "失焦自动隐藏", "", &a.settings.AutoHideOnBlur, func(on bool) {
						a.applyAutoHide(on)
						a.persistSettings()
					})
				})

				// 2. 剪贴板设置
				a.renderSettingsGroup(c, "剪贴板设置", &a.expanded.clipboard, func() {
					a.renderSettingRow(c, "呼出快捷键", "", func() {
						if ui.Select(c, &a.settings.Hotkey, availableHotkeys).Label("呼出快捷键").Changed() {
							chosen := a.settings.Hotkey
							if err := a.switchHotkey(chosen); err != nil {
								if a.shortcut != "" {
									a.settings.Hotkey = a.shortcut
								}
								a.settingsNotice = "快捷键 " + chosen + " 被占用，已保持原设置"
							} else {
								a.settingsNotice = "全局快捷键已更新为 " + chosen
								a.persistSettings()
							}
						}
					})
					a.renderSwitchRow(c, "自动去重", "", &a.settings.Deduplicate, func(bool) {
						a.persistSettings()
					})
					a.renderSwitchRow(c, "记录文件路径", "", &a.settings.CaptureFiles, func(bool) {
						a.persistSettings()
					})
					a.renderChoiceGroup(c, "保留历史上限", []choiceOption{
						{label: "200", a11y: "上限200", active: a.settings.MaxHistory == 200, onClick: func() { a.setMaxHistory(200) }},
						{label: "500", a11y: "上限500", active: a.settings.MaxHistory == 500, onClick: func() { a.setMaxHistory(500) }},
						{label: "1000", a11y: "上限1000", active: a.settings.MaxHistory == 1000, onClick: func() { a.setMaxHistory(1000) }},
						{label: "不限", a11y: "上限不限", active: a.settings.MaxHistory == 0, onClick: func() { a.setMaxHistory(0) }},
					})
					a.renderSwitchRow(c, "点击仅复制", "", &a.settings.ClickCopyOnly, func(bool) {
						a.persistSettings()
					})
					a.renderSwitchRow(c, "粘贴后移到顶部", "", &a.settings.MoveToTopOnPaste, func(bool) {
						a.persistSettings()
					})
				})

				// 3. 界面设置
				a.renderSettingsGroup(c, "界面设置", &a.expanded.appearance, func() {
					a.renderChoiceGroup(c, "窗口材质", []choiceOption{
						{label: "云母", a11y: "材质云母", active: a.settings.BackdropMode == "mica", onClick: func() { a.setBackdropMode("mica") }},
						{label: "毛玻璃", a11y: "材质毛玻璃", active: a.settings.BackdropMode == "acrylic", onClick: func() { a.setBackdropMode("acrylic") }},
						{label: "纯色", a11y: "材质纯色", active: a.settings.BackdropMode == "solid", onClick: func() { a.setBackdropMode("solid") }},
					})
					a.renderChoiceGroup(c, "颜色模式", []choiceOption{
						{label: "跟随系统", a11y: "外观系统", active: a.settings.ColorMode == "system", onClick: func() { a.setColorMode("system") }},
						{label: "浅色", a11y: "外观浅色", active: a.settings.ColorMode == "light", onClick: func() { a.setColorMode("light") }},
						{label: "深色", a11y: "外观深色", active: a.settings.ColorMode == "dark", onClick: func() { a.setColorMode("dark") }},
					})
					a.renderSwitchRow(c, "紧凑卡片模式", "", &a.settings.CompactMode, func(bool) {
						a.persistSettings()
					})
				})

				// 4. 数据管理
				a.renderSettingsGroup(c, "数据管理", &a.expanded.data, func() {
					dataDir := ""
					if a.store != nil {
						dataDir = a.store.BaseDir()
					}
					a.renderSettingRow(c, "本地存储目录", dataDir, func() {
						btn := ui.ButtonBase(c).Height(pillHeight).Padding(0, 12).Radius(radiusGroup).Label("打开目录")
						bg := t.SurfaceHover
						if btn.Hovered() {
							bg = t.SurfacePressed
						}
						btn.Background(bg).Border(1, t.Border.Alpha(0.6)).Transition(colorTransition).
							OnClick(func() {
								if dataDir != "" {
									if err := mygo.Shell.OpenPath(dataDir); err != nil {
										a.settingsNotice = "打开目录失败: " + err.Error()
									}
								}
							}).
							Children(func() {
								ui.Text(c, "打开目录").FontSize(13.5).TextColor(t.Text)
							})
					})
					a.renderConfirmActionRow(c, "清空未置顶记录", "", "clear_unpinned", "清空", "确认清空", func() {
						a.clearUnpinned()
						a.settingsNotice = "已清空未置顶剪贴板记录"
					})
					a.renderConfirmActionRow(c, "清空全部记录", "", "clear_all", "清空全部", "确认清空全部", func() {
						a.clearAll()
						a.settingsNotice = "已清空全部剪贴板记录"
					})
				})

				// 底部恢复初始设置与版本信息
				ui.Column(c).FillWidth().AlignItems(ui.Center).Padding(12, 0, 4, 0).Gap(12).Children(func() {
					resetLabel := "恢复初始设置"
					resetBg, resetBorder, resetText := t.Surface, t.Border, t.Text
					if a.confirmAction == "reset_settings" {
						resetLabel = "再次点击确认恢复"
						resetBg = t.Surface.Mix(t.Danger, 0.1)
						resetBorder = t.Danger
						resetText = t.Danger
					}
					resetBtn := ui.ButtonBase(c).Height(38).Padding(0, 18).Radius(radiusCard).Label("恢复初始设置")
					if resetBtn.Hovered() && a.confirmAction != "reset_settings" {
						resetBg = t.SurfaceHover
						resetBorder = t.Border.Mix(t.TextMuted, 0.35)
					}
					if resetBtn.Pressed() {
						resetBtn.AnimatePose(ui.Pose{Scale: 0.96}, ui.Snappy)
					} else {
						resetBtn.AnimatePose(ui.Pose{}, ui.Snappy)
					}
					resetBtn.Background(resetBg).Border(1, resetBorder).Transition(colorTransition).
						OnClick(func() {
							if a.confirmAction != "reset_settings" {
								a.confirmAction = "reset_settings"
								return
							}
							a.confirmAction = ""
							a.resetSettings()
						}).
						Children(func() {
							ui.Row(c).AlignItems(ui.Center).Gap(7).Children(func() {
								ui.Icon(c, iconRotateCcw).FontSize(16).TextColor(resetText)
								ui.Text(c, resetLabel).FontSize(14).TextColor(resetText)
							})
						})

					ui.Column(c).AlignItems(ui.Center).Gap(3).Children(func() {
						ui.Text(c, "MyTieZ v0.1.0").FontSize(13).TextColor(t.TextMuted)
						ui.Text(c, "让复制与粘贴，保持有序").FontSize(13).TextColor(t.TextMuted)
					})
				})
			})
		})
	})
}

func (a *clipApp) setMaxHistory(val int) {
	a.settings.MaxHistory = val
	a.persistSettings()
	if a.store != nil && val > 0 {
		_ = a.store.EnforceLimit(val)
		a.reload()
	}
}

func (a *clipApp) setBackdropMode(mode string) {
	a.settings.BackdropMode = mode
	a.applyBackdrop()
	a.persistSettings()
}

func (a *clipApp) setColorMode(mode string) {
	a.settings.ColorMode = mode
	a.persistSettings()
}

type choiceOption struct {
	label   string
	a11y    string
	active  bool
	onClick func()
}

func (a *clipApp) renderSettingsGroup(c *ui.Context, title string, expanded *bool, body func()) {
	t := c.Theme()
	ui.Column(c).FillWidth().Radius(radiusGroup).Clip().Background(t.Surface).Border(1, t.Border.Alpha(0.45)).Children(func() {
		p := ui.CollapsibleBase(c, expanded)
		tr := p.Trigger.FillWidth().MinHeight(settingGroupMinH).Padding(0, 16).Justify(ui.SpaceBetween).AlignItems(ui.Center).Label(title)
		bg := t.Surface
		if tr.Hovered() {
			bg = t.SurfaceHover
		}
		tr.Background(bg).Transition(colorTransition).Children(func() {
			ui.Text(c, title).FontSize(15).Bold().TextColor(t.Text)
			ui.Icon(c, iconChevronRight).FontSize(18).TextColor(t.TextMuted).Rotate(90 * p.Progress())
		})

		if panel := p.Panel(func() {
			ui.Box(c).FillWidth().Height(1).Background(t.Text.Alpha(0.05))
			ui.Column(c).FillWidth().Padding(4, 0, 8, 0).Children(body)
		}); panel.Valid() {
			panel.FillWidth()
		}
	})
}

func (a *clipApp) renderSettingRow(c *ui.Context, title, desc string, control func()) {
	t := c.Theme()
	ui.Row(c).FillWidth().MinHeight(44).Padding(8, 16).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(14).FontWeight(500).TextColor(t.Text)
			if desc != "" {
				ui.Text(c, desc).FontSize(12.5).TextColor(t.TextMuted).Wrap().MaxLines(2)
			}
		})
		control()
	})
}

func (a *clipApp) renderChoiceGroup(c *ui.Context, title string, opts []choiceOption) {
	t := c.Theme()
	ui.Column(c).FillWidth().Padding(8, 16).Gap(8).Children(func() {
		ui.Text(c, title).FontSize(14).FontWeight(500).TextColor(t.Text)
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			for _, opt := range opts {
				a.renderChoicePill(c, opt.label, opt.a11y, opt.active, opt.onClick)
			}
		})
	})
}

func (a *clipApp) renderSwitchRow(c *ui.Context, title, desc string, val *bool, onChanged func(bool)) {
	a.renderSettingRow(c, title, desc, func() {
		if ui.Switch(c, val).Label("切换" + title).Changed() {
			a.confirmAction = ""
			if onChanged != nil {
				onChanged(*val)
			}
		}
	})
}

func (a *clipApp) renderChoicePill(c *ui.Context, text, a11yLabel string, active bool, onClick func()) {
	t := c.Theme()
	pill := ui.ButtonBase(c).Grow(1).Height(34).Center().Radius(radiusGroup).Label(a11yLabel)
	bg, border, fg := t.Background, t.Border.Alpha(0.55), t.Text
	if active {
		bg = t.Accent
		border = t.Accent
		fg = t.AccentText
	} else if pill.Hovered() {
		bg = t.SurfaceHover
		border = t.Border
	}
	pill.Background(bg).Border(1, border).Transition(colorTransition).
		OnClick(func() {
			a.confirmAction = ""
			onClick()
		}).
		Children(func() {
			ui.Text(c, text).FontSize(13.5).FontWeight(600).TextColor(fg)
		})
}

func (a *clipApp) renderConfirmActionRow(c *ui.Context, title, desc, actionKey, label, confirmLabel string, onConfirm func()) {
	t := c.Theme()
	confirming := a.confirmAction == actionKey
	text := label
	bg, border, fg := t.SurfaceHover, t.Border, t.Text
	if confirming {
		text = confirmLabel
		bg = t.Surface.Mix(t.Danger, 0.12)
		border = t.Danger
		fg = t.Danger
	}
	a.renderSettingRow(c, title, desc, func() {
		btn := ui.ButtonBase(c).Height(pillHeight).Padding(0, 12).Radius(radiusControl).Label(title)
		if btn.Hovered() && !confirming {
			bg = t.SurfacePressed
		}
		btn.Background(bg).Border(1, border).Transition(colorTransition).
			OnClick(func() {
				if !confirming {
					a.confirmAction = actionKey
					return
				}
				a.confirmAction = ""
				onConfirm()
			}).
			Children(func() {
				ui.Text(c, text).FontSize(14).TextColor(fg)
			})
	})
}
