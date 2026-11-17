package app

import (
	"log"
	"time"

	"mytiez/internal/store"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Zero value keeps every group collapsed by default.
type settingsGroupExpanded struct {
	general    bool
	clipboard  bool
	openApps   bool
	appearance bool
	data       bool
}

func (a *clipApp) settingsView(c *ui.Context) {
	t := c.Theme()

	ui.Column(c.Key("settings-page")).Fill().Children(func() {
		// 设置页顶部导航栏（与主页顶栏同构）
		ui.Column(c).FillWidth().Padding(8, 16, 4, 16).Children(func() {
			ui.Row(c).Height(headerBtnSize).AlignItems(ui.Center).Gap(8).DragWindow().Children(func() {
				a.renderHeaderIconBtn(c, iconBack, "返回", false, false, func() {
					a.cancelHotkeyRecording()
					a.showSettings = false
					a.confirmAction = ""
					a.settingsNotice = ""
				})

				ui.Text(c, "设置").FontSize(18).Bold().TextColor(t.Text)
				ui.Box(c).Grow(1)
				ui.Row(c).AlignItems(ui.Center).Gap(4).Children(func() {
					a.renderWindowActionButtons(c)
				})
			})
		})

		ui.Divider(c)

		ui.Scroll(c.Key("settings-scroll")).Grow(1).Children(func() {
			ui.Column(c).FillWidth().Padding(18, 12, 18, 12).Gap(10).Children(func() {
				if a.settingsNotice != "" {
					ui.Text(c.Key("settings-notice"), a.settingsNotice).FontSize(13).TextColor(t.Accent).
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
					a.renderHotkeyRecorder(c)
					a.renderSwitchRow(c, "自动去重", "", &a.settings.Deduplicate, func(bool) {
						a.persistSettings()
					})
					a.renderSwitchRow(c, "记录文件路径", "", &a.settings.CaptureFiles, func(bool) {
						a.persistSettings()
					})
					a.renderSwitchRow(c, "捕获带格式的文本", "", &a.settings.CaptureRichText, func(bool) {
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

				// 3. 打开方式
				a.renderSettingsGroup(c, "打开方式", &a.expanded.openApps, func() {
					a.renderOpenAppRow(c, "文本", &a.settings.OpenAppText)
					a.renderOpenAppRow(c, "富文本", &a.settings.OpenAppRichText)
					a.renderOpenAppRow(c, "图片", &a.settings.OpenAppImage)
				})

				// 4. 界面设置
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
					a.renderSwitchRow(c, "悬停显示卡片操作", "", &a.settings.HideCardActions, func(bool) {
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
						bg := t.Surface
						if btn.Hovered() {
							bg = t.SurfaceHover
						}
						btn.Background(bg).Border(1, t.Border.Alpha(0.7)).Transition(colorTransition).
							OnClick(func() {
								if dataDir != "" {
									if err := mygo.Shell.OpenPath(dataDir); err != nil {
										a.settingsNotice = "打开目录失败: " + err.Error()
									}
								}
							}).
							Children(func() {
								ui.Text(c, "打开目录").FontSize(13).FontWeight(600).TextColor(t.Text)
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
	ui.Column(c.Key("group-"+title)).FillWidth().Radius(radiusGroup).Clip().Background(t.Surface).Border(1, t.Border.Alpha(0.45)).Children(func() {
		p := ui.CollapsibleBase(c, expanded)
		tr := p.Trigger.FillWidth().MinHeight(settingGroupMinH).Padding(0, 16).Justify(ui.SpaceBetween).AlignItems(ui.Center).Label(title)
		tr.Background(t.Surface).Children(func() {
			ui.Text(c, title).FontSize(15).FontWeight(700).TextColor(t.Text)
			ui.Icon(c, iconChevronRight).FontSize(17).TextColor(t.TextMuted).Rotate(90 * p.Progress())
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
			ui.Text(c, title).FontSize(14).FontWeight(600).TextColor(t.Text)
			if desc != "" {
				ui.Text(c, desc).FontSize(12).TextColor(t.TextMuted).Wrap().MaxLines(2)
			}
		})
		control()
	})
}

func (a *clipApp) renderChoiceGroup(c *ui.Context, title string, opts []choiceOption) {
	t := c.Theme()
	ui.Column(c).FillWidth().Padding(8, 16).Gap(8).Children(func() {
		ui.Text(c, title).FontSize(14).FontWeight(600).TextColor(t.Text)
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			for _, opt := range opts {
				a.renderChoicePill(c, opt.label, opt.a11y, opt.active, opt.onClick)
			}
		})
	})
}

func (a *clipApp) renderSwitchRow(c *ui.Context, title, desc string, val *bool, onChanged func(bool)) {
	a.renderSettingRow(c, title, desc, func() {
		if a.renderSwitch(c, val, "切换"+title) {
			a.confirmAction = ""
			if onChanged != nil {
				onChanged(*val)
			}
		}
	})
}

func (a *clipApp) renderSwitch(c *ui.Context, val *bool, label string) bool {
	t := c.Theme()
	sw := ui.SwitchBase(c, val).Size(38, 21).Radius(10.5).Label(label)
	pos := sw.Animate("knob", b2f(*val), 140*time.Millisecond)

	offBg := t.Border.Mix(t.Accent, 0.08)
	if t.Dark {
		offBg = t.Text.Alpha(0.18)
	}
	onBg := t.Accent
	if sw.Hovered() {
		onBg = t.AccentHover
	}
	if pos > 0 {
		sw.Background(offBg.Mix(onBg, pos))
	} else {
		sw.Background(offBg)
	}
	sw.Draw(func(p *ui.Painter, r ui.Rect) {
		const inset float32 = 2.5
		d := r.H - 2*inset
		knob := ui.Rect{X: r.X + inset + pos*(r.W-r.H), Y: r.Y + inset, W: d, H: d}
		p.Shadow(knob, d/2, 0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.16))
		p.Fill(knob, ui.RGB(255, 255, 255), d/2)
	})
	return sw.Changed()
}

func b2f(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

func (a *clipApp) renderChoicePill(c *ui.Context, text, a11yLabel string, active bool, onClick func()) {
	t := c.Theme()
	pill := ui.ButtonBase(c).Grow(1).Height(32).Center().Radius(radiusGroup).Label(a11yLabel)
	bg, border, fg := t.Surface, t.Border.Alpha(0.65), t.Text
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
			ui.Text(c, text).FontSize(13).FontWeight(600).TextColor(fg)
		})
}

func (a *clipApp) renderConfirmActionRow(c *ui.Context, title, desc, actionKey, label, confirmLabel string, onConfirm func()) {
	t := c.Theme()
	confirming := a.confirmAction == actionKey
	text := label
	bg, border, fg := t.Surface, t.Border.Alpha(0.7), t.Text
	if confirming {
		text = confirmLabel
		bg = t.Surface.Mix(t.Danger, 0.12)
		border = t.Danger
		fg = t.Danger
	}
	a.renderSettingRow(c, title, desc, func() {
		btn := ui.ButtonBase(c).Height(pillHeight).Padding(0, 12).Radius(radiusControl).Label(title)
		if btn.Hovered() && !confirming {
			bg = t.SurfaceHover
			border = t.Border
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
				ui.Text(c, text).FontSize(13).FontWeight(600).TextColor(fg)
			})
	})
}

func (a *clipApp) renderOpenAppRow(c *ui.Context, title string, target *string) {
	t := c.Theme()
	current := ""
	if target != nil {
		current = *target
	}
	a.renderSettingRow(c, title, openAppDisplayName(current), func() {
		ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
			if current != "" {
				resetBtn := ui.ButtonBase(c).Height(pillHeight).Padding(0, 10).Radius(radiusGroup).Label("恢复" + title + "默认打开方式")
				rBg := t.Surface
				if resetBtn.Hovered() {
					rBg = t.SurfaceHover
				}
				resetBtn.Background(rBg).Border(1, t.Border.Alpha(0.7)).Transition(colorTransition).
					OnClick(func() {
						if target != nil {
							*target = ""
							a.settingsNotice = title + "已恢复为系统默认打开方式"
							a.persistSettings()
						}
					}).
					Children(func() {
						ui.Text(c, "恢复默认").FontSize(12).FontWeight(600).TextColor(t.TextMuted)
					})
			}

			pickBtn := ui.ButtonBase(c).Height(pillHeight).Padding(0, 12).Radius(radiusGroup).Label("选择" + title + "打开程序")
			pBg := t.Surface
			if pickBtn.Hovered() {
				pBg = t.SurfaceHover
			}
			pickBtn.Background(pBg).Border(1, t.Border.Alpha(0.7)).Transition(colorTransition).
				OnClick(func() {
					a.pickOpenApp(title, target)
				}).
				Children(func() {
					ui.Text(c, "选择程序").FontSize(13).FontWeight(600).TextColor(t.Text)
				})
		})
	})
}

func (a *clipApp) renderHotkeyRecorder(c *ui.Context) {
	t := c.Theme()
	defaultHotkey := store.DefaultSettings().Hotkey

	if a.hotkeyFocusPending {
		a.hotkeyFocusPending = false
		a.hotkeyHandle.Focus(c)
	} else if a.recordingHotkey && !a.hotkeyHandle.Focused(c) {
		a.cancelHotkeyRecording()
	}

	desc := "点击右侧按钮后按下组合键"
	if a.recordingHotkey {
		desc = "按下目标组合键，按 Esc 取消"
	}

	a.renderSettingRow(c, "呼出快捷键", desc, func() {
		ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
			if a.recordingHotkey {
				cancelBtn := ui.ButtonBase(c).Height(pillHeight).Padding(0, 10).Radius(radiusGroup).Label("取消录制快捷键")
				cBg := t.Surface
				if cancelBtn.Hovered() {
					cBg = t.SurfaceHover
				}
				cancelBtn.Background(cBg).Border(1, t.Border.Alpha(0.7)).Transition(colorTransition).
					OnClick(func() {
						a.cancelHotkeyRecording()
						a.settingsNotice = ""
					}).
					Children(func() {
						ui.Text(c, "取消").FontSize(12).FontWeight(600).TextColor(t.TextMuted)
					})
			} else if a.settings.Hotkey != defaultHotkey {
				resetBtn := ui.ButtonBase(c).Height(pillHeight).Padding(0, 10).Radius(radiusGroup).Label("恢复默认快捷键")
				rBg := t.Surface
				if resetBtn.Hovered() {
					rBg = t.SurfaceHover
				}
				resetBtn.Background(rBg).Border(1, t.Border.Alpha(0.7)).Transition(colorTransition).
					OnClick(func() {
						a.applyHotkey(defaultHotkey)
					}).
					Children(func() {
						ui.Text(c, "恢复默认").FontSize(12).FontWeight(600).TextColor(t.TextMuted)
					})
			}

			labelText := a.settings.Hotkey
			if labelText == "" {
				labelText = defaultHotkey
			}
			if a.recordingHotkey {
				labelText = recordingHotkeyLabel(a.hotkeyMods)
			}

			recBtn := ui.ButtonBase(c.Key("hotkey-recorder")).Bind(&a.hotkeyHandle).
				Height(pillHeight).MinWidth(132).Padding(0, 12).Radius(radiusGroup).
				Center().Label("录制呼出快捷键").
				HandleInput(a.handleHotkeyInput)

			bg, border, borderW, fg := t.Surface, t.Border.Alpha(0.7), float32(1), t.Text
			if a.recordingHotkey {
				bg = t.Surface.Mix(t.Accent, 0.12)
				border = t.Accent
				borderW = 1.5
				fg = t.Accent
			} else if recBtn.Hovered() {
				bg = t.SurfaceHover
				border = t.Border
			}

			recBtn.Background(bg).Border(borderW, border).Transition(colorTransition).
				OnClick(func() {
					a.startHotkeyRecording()
				}).
				Children(func() {
					ui.Text(c, labelText).FontSize(13).FontWeight(600).TextColor(fg)
				})
		})
	})
}
