package app

import (
	"log"
	"time"

	"mytiez/internal/store"
	"mytiez/internal/sys"
	"mytiez/locales"

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
	msgs := locales.In(c.Locale())

	ui.Column(c.Key("settings-page")).Fill().Children(func() {
		// 设置页顶部导航栏（与主页顶栏同构，下方带左右留白的细分割线）
		ui.Column(c).FillWidth().Padding(10, 16, 8, 16).Children(func() {
			ui.Row(c).Height(headerBtnSize).AlignItems(ui.Center).Gap(8).DragWindow().Children(func() {
				a.renderHeaderIconBtn(c, iconBack, msgs.HeaderBack(), false, false, func() {
					a.cancelHotkeyRecording()
					a.showSettings = false
					a.confirmAction = ""
					a.settingsNotice = ""
				})

				ui.Text(c, msgs.SettingsTitle()).FontSize(17).Bold().TextColor(t.Text)
				ui.Box(c).Grow(1)
				ui.Row(c).AlignItems(ui.Center).Gap(4).Children(func() {
					a.renderWindowActionButtons(c)
				})
			})
		})

		ui.Box(c).FillWidth().Padding(0, 14, 0, 14).Children(func() {
			ui.Box(c).FillWidth().Height(1).Background(t.Border.Alpha(0.45))
		})

		ui.Scroll(c.Key("settings-scroll")).Grow(1).Children(func() {
			ui.Column(c).FillWidth().Padding(22, 14, 18, 14).Gap(10).Children(func() {
				if a.settingsNotice != "" {
					ui.Text(c.Key("settings-notice"), a.settingsNotice).FontSize(13).TextColor(t.Accent).
						FillWidth().Padding(10, 14).Radius(radiusGroup).
						Background(t.Surface.Mix(t.Accent, 0.1)).
						Border(1, t.Accent.Alpha(0.4))
				}

				// 1. 常规设置
				a.renderSettingsGroup(c, msgs.SettingsGroupGeneral(), &a.expanded.general, func() {
					a.renderSwitchRow(c, msgs.SettingsAutostart(), "", &a.settings.AutoStart, func(on bool) {
						if err := mygo.App.SetOpenAtLogin(on); err != nil {
							log.Printf("set open at login: %v", err)
							a.settings.AutoStart = !on
							a.settingsNotice = msgs.SettingsAutostartErr(err.Error())
						}
						a.persistSettings()
					})
					a.renderSwitchRow(c, msgs.SettingsSilentStart(), "", &a.settings.SilentStart, func(bool) {
						a.persistSettings()
					})
					a.renderSwitchRow(c, msgs.SettingsFollowMouse(), "", &a.settings.FollowMouse, func(bool) {
						a.persistSettings()
					})
					a.renderSwitchRow(c, msgs.SettingsAutoHide(), "", &a.settings.AutoHideOnBlur, func(on bool) {
						a.applyAutoHide(on)
						a.persistSettings()
					})
					a.renderSwitchRow(c, msgs.SettingsPinSearchBox(), "", &a.settings.PinSearchBox, func(on bool) {
						if on {
							a.controlsCollapsed = false
						}
						a.persistSettings()
					})
					a.renderSwitchRow(c, msgs.SettingsScrollTopBtn(), "", &a.settings.ScrollTopButton, func(bool) {
						a.persistSettings()
					})
					a.renderSettingRow(c, msgs.SettingsRestartAdmin(), msgs.SettingsRestartAdminDesc(), func() {
						btn := ui.ButtonBase(c).Height(pillHeight).Padding(0, 12).Radius(radiusGroup).Label(msgs.SettingsRestartAdmin())
						bg := t.Surface
						if btn.Hovered() {
							bg = t.SurfaceHover
						}
						btn.Background(bg).Border(1, t.Border.Alpha(0.7)).Transition(colorTransition).
							OnClick(func() {
								if err := sys.RestartAsAdmin(); err != nil {
									a.settingsNotice = msgs.SettingsRestartAdminErr(err.Error())
									return
								}
								mygo.App.Quit()
							}).
							Children(func() {
								ui.Text(c, msgs.SettingsRestartBtn()).FontSize(13).FontWeight(600).TextColor(t.Text)
							})
					})
				})

				// 2. 剪贴板设置
				a.renderSettingsGroup(c, msgs.SettingsGroupClipboard(), &a.expanded.clipboard, func() {
					a.renderHotkeyRecorder(c)
					a.renderSwitchRow(c, msgs.SettingsDeduplicate(), "", &a.settings.Deduplicate, func(bool) {
						a.persistSettings()
					})
					a.renderSwitchRow(c, msgs.SettingsCaptureFiles(), "", &a.settings.CaptureFiles, func(bool) {
						a.persistSettings()
					})
					a.renderSwitchRow(c, msgs.SettingsCaptureRichText(), "", &a.settings.CaptureRichText, func(bool) {
						a.persistSettings()
					})
					a.renderChoiceGroup(c, msgs.SettingsMaxHistory(), []choiceOption{
						{label: "200", a11y: msgs.SettingsLimit200(), active: a.settings.MaxHistory == 200, onClick: func() { a.setMaxHistory(200) }},
						{label: "500", a11y: msgs.SettingsLimit500(), active: a.settings.MaxHistory == 500, onClick: func() { a.setMaxHistory(500) }},
						{label: "1000", a11y: msgs.SettingsLimit1000(), active: a.settings.MaxHistory == 1000, onClick: func() { a.setMaxHistory(1000) }},
						{label: msgs.SettingsUnlimited(), a11y: msgs.SettingsLimitUnlimited(), active: a.settings.MaxHistory == 0, onClick: func() { a.setMaxHistory(0) }},
					})
					a.renderSwitchRow(c, msgs.SettingsClickCopyOnly(), "", &a.settings.ClickCopyOnly, func(bool) {
						a.persistSettings()
					})
					a.renderSwitchRow(c, msgs.SettingsMoveToTop(), "", &a.settings.MoveToTopOnPaste, func(bool) {
						a.persistSettings()
					})
					a.renderSwitchRow(c, msgs.SettingsDeleteAfterPaste(), "", &a.settings.DeleteAfterPaste, func(bool) {
						a.persistSettings()
					})
					a.renderPasteMethodGroup(c, msgs)
					a.renderChoiceGroup(c, msgs.SettingsQuickPaste(), []choiceOption{
						{label: msgs.SettingsQuickPasteOff(), a11y: msgs.SettingsQuickPasteOffA11y(), active: a.settings.QuickPasteModifier == "none", onClick: func() { a.setQuickPasteModifier("none") }},
						{label: "Alt+1~9", a11y: msgs.SettingsQuickPasteAltA11y(), active: a.settings.QuickPasteModifier == "alt", onClick: func() { a.setQuickPasteModifier("alt") }},
						{label: "Ctrl+1~9", a11y: msgs.SettingsQuickPasteCtrlA11y(), active: a.settings.QuickPasteModifier == "ctrl", onClick: func() { a.setQuickPasteModifier("ctrl") }},
					})
				})

				// 3. 界面设置
				a.renderSettingsGroup(c, msgs.SettingsGroupAppearance(), &a.expanded.appearance, func() {
					curLocale := c.Locale()
					a.renderChoiceGroup(c, msgs.SettingsLanguage(), []choiceOption{
						{label: msgs.SettingsLangZh(), a11y: msgs.SettingsLangZh(), active: curLocale != "zh-Hant" && curLocale != "en", onClick: func() { a.setLanguage("zh-Hans") }},
						{label: msgs.SettingsLangTw(), a11y: msgs.SettingsLangTw(), active: curLocale == "zh-Hant", onClick: func() { a.setLanguage("zh-Hant") }},
						{label: msgs.SettingsLangEn(), a11y: msgs.SettingsLangEn(), active: curLocale == "en", onClick: func() { a.setLanguage("en") }},
					})
					a.renderChoiceGroup(c, msgs.SettingsBackdrop(), []choiceOption{
						{label: msgs.SettingsBackdropSolid(), a11y: msgs.SettingsBackdropSolidA11y(), active: a.settings.BackdropMode == "solid", onClick: func() { a.setBackdropMode("solid") }},
						{label: msgs.SettingsBackdropMica(), a11y: msgs.SettingsBackdropMicaA11y(), active: a.settings.BackdropMode == "mica", onClick: func() { a.setBackdropMode("mica") }},
						{label: msgs.SettingsBackdropAcrylic(), a11y: msgs.SettingsBackdropAcrylicA11y(), active: a.settings.BackdropMode == "acrylic", onClick: func() { a.setBackdropMode("acrylic") }},
					})
					a.renderChoiceGroup(c, msgs.SettingsColorMode(), []choiceOption{
						{label: msgs.SettingsColorSystem(), a11y: msgs.SettingsColorSystemA11y(), active: a.settings.ColorMode == "system", onClick: func() { a.setColorMode("system") }},
						{label: msgs.SettingsColorLight(), a11y: msgs.SettingsColorLightA11y(), active: a.settings.ColorMode == "light", onClick: func() { a.setColorMode("light") }},
						{label: msgs.SettingsColorDark(), a11y: msgs.SettingsColorDarkA11y(), active: a.settings.ColorMode == "dark", onClick: func() { a.setColorMode("dark") }},
					})
					a.renderSwitchRow(c, msgs.SettingsCompactMode(), "", &a.settings.CompactMode, func(bool) {
						a.persistSettings()
					})
					a.renderSwitchRow(c, msgs.SettingsHideCardActions(), "", &a.settings.HideCardActions, func(bool) {
						a.persistSettings()
					})
					a.renderSwitchRow(c, msgs.SettingsShowSourceIcon(), "", &a.settings.ShowSourceIcon, func(bool) {
						a.persistSettings()
					})
				})

				// 4. 默认打开程序
				a.renderSettingsGroup(c, msgs.SettingsGroupOpenApps(), &a.expanded.openApps, func() {
					a.renderOpenAppRow(c, msgs.TabText(), &a.settings.OpenAppText)
					a.renderOpenAppRow(c, msgs.TabRichText(), &a.settings.OpenAppRichText)
					a.renderOpenAppRow(c, msgs.TabImage(), &a.settings.OpenAppImage)
				})

				// 5. 数据管理
				a.renderSettingsGroup(c, msgs.SettingsGroupData(), &a.expanded.data, func() {
					dataDir := ""
					if a.store != nil {
						dataDir = a.store.BaseDir()
					}
					a.renderSettingRow(c, msgs.SettingsDataDir(), dataDir, func() {
						btn := ui.ButtonBase(c).Height(pillHeight).Padding(0, 12).Radius(radiusGroup).Label(msgs.SettingsOpenDir())
						bg := t.Surface
						if btn.Hovered() {
							bg = t.SurfaceHover
						}
						btn.Background(bg).Border(1, t.Border.Alpha(0.7)).Transition(colorTransition).
							OnClick(func() {
								if dataDir != "" {
									if err := mygo.Shell.OpenPath(dataDir); err != nil {
										a.settingsNotice = msgs.SettingsOpenDirErr(err.Error())
									}
								}
							}).
							Children(func() {
								ui.Text(c, msgs.SettingsOpenDir()).FontSize(13).FontWeight(600).TextColor(t.Text)
							})
					})
					a.renderConfirmActionRow(c, msgs.SettingsClearUnpinned(), "", "clear_unpinned", msgs.SettingsClearBtn(), msgs.SettingsClearConfirm(), func() {
						a.clearUnpinned()
						a.settingsNotice = msgs.SettingsClearUnpinnedDone()
					})
					a.renderConfirmActionRow(c, msgs.SettingsClearAll(), "", "clear_all", msgs.SettingsClearAllBtn(), msgs.SettingsClearAllConfirm(), func() {
						a.clearAll()
						a.settingsNotice = msgs.SettingsClearAllDone()
					})
				})

				// 底部恢复初始设置与版本信息
				ui.Column(c).FillWidth().AlignItems(ui.Center).Padding(12, 0, 4, 0).Gap(12).Children(func() {
					resetLabel := msgs.SettingsResetAll()
					resetBg, resetBorder, resetText := t.Surface, t.Border, t.Text
					if a.confirmAction == "reset_settings" {
						resetLabel = msgs.SettingsResetAllConfirm()
						resetBg = t.Surface.Mix(t.Danger, 0.1)
						resetBorder = t.Danger
						resetText = t.Danger
					}
					resetBtn := ui.ButtonBase(c).Height(38).Padding(0, 18).Radius(radiusCard).Label(msgs.SettingsResetAll())
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
						ui.Text(c, msgs.SettingsTagline()).FontSize(13).TextColor(t.TextMuted)
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

func (a *clipApp) setQuickPasteModifier(mod string) {
	a.settings.QuickPasteModifier = mod
	a.persistSettings()
}

func (a *clipApp) setPasteMethod(method string) {
	a.settings.PasteMethod = method
	a.persistSettings()
}

func (a *clipApp) renderPasteMethodGroup(c *ui.Context, msgs locales.Messages) {
	t := c.Theme()
	desc := msgs.SettingsPasteMethodShiftInsertDesc()
	switch a.settings.PasteMethod {
	case "ctrl_v":
		desc = msgs.SettingsPasteMethodCtrlVDesc()
	case "game_mode":
		desc = msgs.SettingsPasteMethodGameModeDesc()
	}
	opts := []choiceOption{
		{label: msgs.SettingsPasteMethodShiftInsert(), a11y: msgs.SettingsPasteMethodShiftInsert(), active: a.settings.PasteMethod == "shift_insert", onClick: func() { a.setPasteMethod("shift_insert") }},
		{label: msgs.SettingsPasteMethodCtrlV(), a11y: msgs.SettingsPasteMethodCtrlV(), active: a.settings.PasteMethod == "ctrl_v", onClick: func() { a.setPasteMethod("ctrl_v") }},
		{label: msgs.SettingsPasteMethodGameMode(), a11y: msgs.SettingsPasteMethodGameMode(), active: a.settings.PasteMethod == "game_mode", onClick: func() { a.setPasteMethod("game_mode") }},
	}
	ui.Column(c).FillWidth().Padding(8, 16).Gap(8).Children(func() {
		ui.Column(c).Gap(2).Children(func() {
			ui.Text(c, msgs.SettingsPasteMethod()).FontSize(14).FontWeight(600).TextColor(t.Text)
			ui.Text(c, desc).FontSize(12).TextColor(t.TextMuted).Wrap().MaxLines(2)
		})
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			for _, opt := range opts {
				a.renderChoicePill(c, opt.label, opt.a11y, opt.active, opt.onClick)
			}
		})
	})
}

func (a *clipApp) setLanguage(locale string) {
	if mygo.App.IsReady() {
		mygo.App.SetLocale(locale)
	}
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
	shadowAlpha := float32(0.05)
	if t.Dark {
		shadowAlpha = 0.22
	}
	ui.Column(c.Key("group-"+title)).FillWidth().Radius(radiusCard).
		Background(t.Surface).Border(1, t.Border.Alpha(0.38)).
		Shadow(0, 2, 8, 0, ui.RGBA(0, 0, 0, shadowAlpha)).
		Children(func() {
			p := ui.CollapsibleBase(c, expanded)
			tr := p.Trigger.FillWidth().MinHeight(50).Padding(0, 16).Radius(radiusCard).
				Justify(ui.SpaceBetween).AlignItems(ui.Center).Label(title)
			tr.Background(t.Surface).Children(func() {
				ui.Text(c, title).FontSize(14.5).FontWeight(700).TextColor(t.Text)
				ui.Icon(c, iconChevronRight).FontSize(15).TextColor(t.TextMuted).Rotate(90 * p.Progress())
			})

			if panel := p.Panel(func() {
				ui.Box(c).FillWidth().Height(1).Background(t.Border.Alpha(0.35))
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
	msgs := locales.In(c.Locale())
	a.renderSettingRow(c, title, desc, func() {
		if a.renderSwitch(c, val, msgs.SettingsSwitchA11y(title)) {
			a.confirmAction = ""
			if onChanged != nil {
				onChanged(*val)
			}
		}
	})
}

func (a *clipApp) renderSwitch(c *ui.Context, val *bool, label string) bool {
	t := c.Theme()
	sw := ui.SwitchBase(c, val).Size(46, 25).Radius(12.5).Label(label)
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
		const inset float32 = 3
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
	msgs := locales.In(c.Locale())
	current := ""
	if target != nil {
		current = *target
	}
	a.renderSettingRow(c, title, openAppDisplayName(msgs, current), func() {
		ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
			if current != "" {
				resetBtn := ui.ButtonBase(c).Height(pillHeight).Padding(0, 10).Radius(radiusGroup).Label(msgs.OpenAppResetA11y(title))
				rBg := t.Surface
				if resetBtn.Hovered() {
					rBg = t.SurfaceHover
				}
				resetBtn.Background(rBg).Border(1, t.Border.Alpha(0.7)).Transition(colorTransition).
					OnClick(func() {
						if target != nil {
							*target = ""
							a.settingsNotice = msgs.OpenAppResetNotice(title)
							a.persistSettings()
						}
					}).
					Children(func() {
						ui.Text(c, msgs.OpenAppResetBtn()).FontSize(12).FontWeight(600).TextColor(t.TextMuted)
					})
			}

			pickBtn := ui.ButtonBase(c).Height(pillHeight).Padding(0, 12).Radius(radiusGroup).Label(msgs.OpenAppPickA11y(title))
			pBg := t.Surface
			if pickBtn.Hovered() {
				pBg = t.SurfaceHover
			}
			pickBtn.Background(pBg).Border(1, t.Border.Alpha(0.7)).Transition(colorTransition).
				OnClick(func() {
					a.pickOpenApp(title, target)
				}).
				Children(func() {
					ui.Text(c, msgs.OpenAppPickBtn()).FontSize(13).FontWeight(600).TextColor(t.Text)
				})
		})
	})
}

func (a *clipApp) renderHotkeyRecorder(c *ui.Context) {
	t := c.Theme()
	msgs := locales.In(c.Locale())
	defaultHotkey := store.DefaultSettings().Hotkey

	if a.hotkeyFocusPending {
		a.hotkeyFocusPending = false
		a.hotkeyHandle.Focus(c)
	} else if a.recordingHotkey && !a.hotkeyHandle.Focused(c) {
		a.cancelHotkeyRecording()
	}

	desc := msgs.SettingsHotkeyDesc()
	if a.recordingHotkey {
		desc = msgs.SettingsHotkeyDescRecording()
	}

	a.renderSettingRow(c, msgs.SettingsHotkey(), desc, func() {
		ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
			if a.recordingHotkey {
				cancelBtn := ui.ButtonBase(c).Height(pillHeight).Padding(0, 10).Radius(radiusGroup).Label(msgs.SettingsHotkeyCancelA11y())
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
						ui.Text(c, msgs.SettingsHotkeyCancel()).FontSize(12).FontWeight(600).TextColor(t.TextMuted)
					})
			} else if a.settings.Hotkey != defaultHotkey {
				resetBtn := ui.ButtonBase(c).Height(pillHeight).Padding(0, 10).Radius(radiusGroup).Label(msgs.SettingsHotkeyResetA11y())
				rBg := t.Surface
				if resetBtn.Hovered() {
					rBg = t.SurfaceHover
				}
				resetBtn.Background(rBg).Border(1, t.Border.Alpha(0.7)).Transition(colorTransition).
					OnClick(func() {
						a.applyHotkey(defaultHotkey)
					}).
					Children(func() {
						ui.Text(c, msgs.SettingsHotkeyReset()).FontSize(12).FontWeight(600).TextColor(t.TextMuted)
					})
			}

			labelText := a.settings.Hotkey
			if labelText == "" {
				labelText = defaultHotkey
			}
			if a.recordingHotkey {
				labelText = recordingHotkeyLabel(msgs, a.hotkeyMods)
			}

			recBtn := ui.ButtonBase(c.Key("hotkey-recorder")).Bind(&a.hotkeyHandle).
				Height(pillHeight).MinWidth(132).Padding(0, 12).Radius(radiusGroup).
				Center().Label(msgs.SettingsHotkeyRecA11y()).
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
