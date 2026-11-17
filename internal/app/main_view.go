package app

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"mytiez/internal/store"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	searchRevealThreshold float32 = 16
	wheelDeltaMin         float32 = 6
	hideScrollOffset      float32 = 4
	atTopEpsilon          float32 = 12
	topSettleDuration             = 72 * time.Millisecond
)

func (a *clipApp) renderHeaderIconBtn(c *ui.Context, icon *ui.SVG, label string, active, danger bool, onClick func()) {
	t := c.Theme()
	btn := ui.ButtonBase(c).Size(headerBtnSize, headerBtnSize).Center().Radius(radiusControl).Label(label)
	bg := t.Surface
	border := t.Border
	fg := t.TextMuted
	if active {
		bg = t.Surface.Mix(t.Accent, 0.14)
		border = t.Accent
		fg = t.Accent
	} else if btn.Hovered() {
		if danger {
			bg = t.Surface.Mix(t.Danger, 0.14)
			border = t.Danger
			fg = t.Danger
		} else {
			bg = t.SurfaceHover
			border = t.Border.Mix(t.TextMuted, 0.35)
			fg = t.Text
		}
	}
	if btn.Pressed() {
		btn.AnimatePose(ui.Pose{Scale: 0.92}, ui.Snappy)
	} else {
		btn.AnimatePose(ui.Pose{}, ui.Snappy)
	}
	btn.Background(bg).Border(1, border).Transition(colorTransition).
		OnClick(onClick).
		Children(func() { ui.Icon(c, icon).FontSize(19).TextColor(fg) })
}

func (a *clipApp) renderWindowActionButtons(c *ui.Context) {
	a.renderHeaderIconBtn(c, iconPin, "固定窗口", a.winPinned, false, func() {
		a.toggleWindowPin()
	})
	a.renderHeaderIconBtn(c, iconClear, "隐藏窗口", false, true, func() {
		if a.panel != nil {
			a.panel.Hide()
		}
	})
}

func (a *clipApp) view(c *ui.Context) {
	c.SetTheme(a.resolveTheme(c))
	t := c.Theme()
	if a.usingVibrancy(c) {
		c.Root().Background(ui.Transparent)
	} else {
		c.Root().Background(t.Background)
	}

	if a.showSettings {
		if c.Shortcut(0, ui.KeyEscape) {
			if a.confirmAction != "" {
				a.confirmAction = ""
			} else {
				a.showSettings = false
				a.settingsNotice = ""
			}
		}
		a.settingsView(c)
		return
	}

	if a.query != "" && c.Shortcut(0, ui.KeyEscape) {
		a.query = ""
		a.reload()
	} else if a.selected >= 0 && c.Shortcut(0, ui.KeyEscape) {
		a.selected = -1
	} else if c.Shortcut(0, ui.KeyEscape) && a.panel != nil {
		a.panel.Hide()
	}
	if c.Shortcut(0, ui.KeyDown) {
		a.moveSelection(1)
	}
	if c.Shortcut(0, ui.KeyUp) {
		a.moveSelection(-1)
	}
	enterPressed := c.Shortcut(0, ui.KeyEnter)

	page := ui.Column(c.Key("main-page")).Fill()
	page.Children(func() {
		// 顶部标题栏（常驻固定）
		ui.Column(c).Padding(10, 16).Children(func() {
			ui.Row(c).Height(headerBtnSize).AlignItems(ui.Center).Gap(6).DragWindow().Children(func() {
				ui.Text(c, "MyTieZ").FontSize(22).Bold().TextColor(t.Text)
				ui.Box(c).Grow(1)

				a.renderHeaderIconBtn(c, iconTrash, "清空未置顶", false, false, func() {
					a.clearUnpinned()
				})
				a.renderHeaderIconBtn(c, iconSettings, "设置", false, false, func() {
					a.showSettings = true
					a.confirmAction = ""
					a.settingsNotice = ""
				})
				a.renderWindowActionButtons(c)
			})
		})

		// 搜索框与分类筛选栏（离开顶部时收起，滚回最顶部再向上滚展开）
		a.updateControlsCollapse(c)
		targetCtrl := float32(1)
		if a.controlsCollapsed {
			targetCtrl = 0
		}
		ctrlWrap := ui.Column(c.Key("header-controls")).FillWidth()
		p := ctrlWrap.Animate("open", targetCtrl, 180*time.Millisecond)
		if p < 1 {
			const controlsFullHeight float32 = 90 // 2(top pad) + 46(search) + 8(gap) + 26(tabs) + 8(bottom pad)
			ctrlWrap.ClipY().Height(p * controlsFullHeight).Opacity(p)
		}
		if p > 0 {
			ctrlWrap.Padding(2, 16, 8, 16).Gap(8).Children(func() {
				searchBorder := t.Border
				if a.searchHandle.Focused(c) {
					searchBorder = t.Accent
				}
				ui.Row(c).Height(searchBoxHeight).Padding(0, 12).Gap(8).AlignItems(ui.Center).Radius(radiusControl).
					Background(t.Surface).Border(1, searchBorder).Transition(colorTransition).
					Children(func() {
						ui.Icon(c, iconSearch).FontSize(18).TextColor(t.TextMuted)
						if ui.TextInputBase(c, &a.query).Bind(&a.searchHandle).
							Grow(1).FontSize(14).FontWeight(500).
							Placeholder("搜索剪贴板…").Label("搜索剪贴板").Changed() {
							a.reload()
						}
						if a.query != "" {
							ui.ButtonBase(c).Size(24, 24).Center().Radius(radiusMicro).Label("清除搜索").OnClick(func() {
								a.query = ""
								a.reload()
								a.searchHandle.Focus(c)
							}).Children(func() {
								ui.Icon(c, iconClear).FontSize(16).TextColor(t.TextMuted)
							})
						}
					})

				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					tabs := []string{"文本", "图片", "文件"}
					for i, label := range tabs {
						tabIdx := i + 1
						active := a.filterIdx == tabIdx
						tab := ui.ButtonBase(c).Height(filterPillHeight).Padding(0, 9).Radius(radiusMicro).Label(label)
						tabBg, tabBorder, tabText := t.Surface, t.Border, t.TextMuted
						if active {
							tabBg = t.Surface.Mix(t.Accent, 0.1)
							tabBorder = t.Accent
							tabText = t.Accent
						} else if tab.Hovered() {
							tabBg = t.SurfaceHover
							tabBorder = t.Border.Mix(t.TextMuted, 0.35)
							tabText = t.Text
						}
						tab.Background(tabBg).Border(1, tabBorder).Transition(colorTransition).
							OnClick(func() {
								if a.filterIdx == tabIdx {
									a.filterIdx = 0
								} else {
									a.filterIdx = tabIdx
								}
								a.reload()
							}).
							Children(func() {
								ui.Text(c, label).FontSize(12).FontWeight(600).TextColor(tabText)
							})
					}
				})
			})
		}

		ui.Divider(c)

		// 中部虚拟列表（直接铺满到底，无常驻底栏）
		a.list.Selected = nil
		a.list.MoveAnimation = &ui.Snappy
		a.list.Key = func(i int) any { return a.items[i].ID }
		a.list.Label = func(i int) string { return a.items[i].Preview }

		ui.Column(c).Grow(1).HandleInput(a.handleListWheel).Children(func() {
			list := ui.List(c, &a.list, len(a.items)).TrackScroll(&a.listScroll).
				Grow(1).Padding(6, 16, 12, 16).Gap(4).
				Rows(func(row ui.ListRow) {
					a.renderCard(row)
				}).
				Children(func() {
					if len(a.items) == 0 {
						ui.Column(c).Fill().Center().Padding(32).Gap(6).Children(func() {
							ui.Text(c, "暂无剪贴板记录").FontSize(15).TextColor(t.TextMuted)
							ui.Text(c, "复制文本、图片或文件后将自动出现在这里").FontSize(13).TextColor(t.TextMuted)
						})
					}
				})

			if enterPressed || list.Submitted() {
				a.pasteSelected()
			}
		})

		if a.statusMsg != "" && time.Now().Before(a.statusUntil) {
			ui.Row(c).Padding(6, 14).Justify(ui.Center).Background(t.Surface).Border(1, t.Border).Children(func() {
				ui.Text(c, a.statusMsg).FontSize(12).TextColor(t.Accent)
			})
		}
	})
}

func (a *clipApp) renderCard(row ui.ListRow) {
	c := row.Context
	t := c.Theme()
	item := a.items[row.Index]
	selected := a.selected >= 0 && row.Index == a.selected

	bg := t.Surface
	borderColor := t.Border
	var borderWidth float32 = 1
	switch {
	case selected:
		bg = t.Surface.Mix(t.Text, 0.035)
		borderColor = t.Accent
		borderWidth = 1.5
	case row.Pressed():
		bg = t.SurfacePressed
		borderColor = t.Border.Mix(t.TextMuted, 0.4)
	case row.Hovered():
		borderColor = t.Border.Mix(t.TextMuted, 0.35)
	}

	var padTop, padX, padBottom, gap float32 = 10, 12, 11, 0
	maxLines := 4
	bodySize := float32(14)
	if a.settings.CompactMode {
		padTop, padX, padBottom, gap = 6, 10, 6, 2
		maxLines = 1
		bodySize = 13
	}

	card := ui.Column(c).Padding(padTop, padX, padBottom, padX).Gap(gap).Radius(radiusCard).
		Background(bg).Border(borderWidth, borderColor).Transition(colorTransition).
		Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(4).Children(func() {
				kindIcon, _ := kindMeta(item.Kind)
				if appBmp := a.getOrLoadAppIcon(item.SourcePath); appBmp != nil {
					ui.Image(c, appBmp).Size(14, 14).Fit(ui.Contain).Radius(3)
				} else {
					ui.Icon(c, kindIcon).FontSize(13).TextColor(t.TextMuted)
				}
				sourceName := item.SourceApp
				if sourceName == "" {
					sourceName = "Unknown"
				}
				ui.Text(c, sourceName).FontSize(11.5).FontWeight(700).TextColor(t.TextMuted).SingleLine()

				if item.Pinned {
					ui.Text(c, "置顶").FontSize(11).FontWeight(600).TextColor(t.Accent).
						Padding(1, 5).Radius(radiusMicro).Background(t.Surface.Mix(t.Accent, 0.12))
				}

				ui.Box(c).Grow(1)

				a.renderCardActionBtn(c, iconPaste, "粘贴", false, func() {
					a.selected = row.Index
					a.pasteSelected()
				})
				a.renderCardActionBtn(c, iconPin, "置顶", item.Pinned, func() {
					a.togglePin(item.ID)
				})
				a.renderCardActionBtn(c, iconClear, "删除", false, func() {
					a.deleteItem(item.ID)
				})

				ui.Text(c, relativeTime(item.UpdatedAt)).Width(48).TextAlign(ui.End).
					FontSize(11).FontWeight(400).TextColor(t.TextMuted).SingleLine()
			})

			if !a.settings.CompactMode {
				ui.Box(c).FillWidth().Height(1).Background(t.Text.Alpha(0.045))
				ui.Box(c).Height(3)
			}

			switch item.Kind {
			case store.KindText:
				ui.Text(c, item.Preview).FontSize(bodySize).FontWeight(400).TextColor(t.Text).Wrap().MaxLines(maxLines)
			case store.KindImage:
				a.renderImagePreview(c, item)
			case store.KindFile:
				a.renderFilesPreview(c, item)
			}
		})

	if card.RowClicked() {
		a.selected = row.Index
	}
	if card.RowDoubleClicked() {
		a.selected = row.Index
		a.pasteSelected()
	}
}

func (a *clipApp) canAutoCollapseControls() bool {
	return len(a.items) > 0 && a.query == "" && a.filterIdx == 0
}

func (a *clipApp) updateControlsCollapse(c *ui.Context) {
	if !a.canAutoCollapseControls() {
		a.controlsCollapsed = false
		a.atTop = true
		a.upScrollAccum = 0
		return
	}
	offset := a.listScroll.Y
	nowAtTop := offset <= atTopEpsilon
	if !a.atTop && nowAtTop {
		// 刚滑回顶部：记下到达时间进入短暂冷却，避免惯性余波立刻弹出搜索栏
		a.topReachedAt = c.Now()
		a.upScrollAccum = 0
	}
	a.atTop = nowAtTop
	if offset > hideScrollOffset {
		a.controlsCollapsed = true
		a.upScrollAccum = 0
	}
}

func (a *clipApp) handleListWheel(ev ui.InputEvent) bool {
	if ev.Kind != ui.InputScroll || !a.canAutoCollapseControls() || ev.DY == 0 {
		return false
	}

	offset := a.listScroll.Y
	atTop := offset <= atTopEpsilon
	scrollingDown := ev.DY > 0
	scrollingUp := ev.DY < 0

	// 向下滚动且搜索栏当前展开：立即收起；若还在顶部，拦截本次滚动以免列表同时下跳
	if scrollingDown && !a.controlsCollapsed {
		if !atTop || a.listScroll.MaxY > 0 {
			a.controlsCollapsed = true
			a.upScrollAccum = 0
			if atTop {
				return true
			}
		}
		return false
	}

	absDY := ev.DY
	if absDY < 0 {
		absDY = -absDY
	}
	if absDY <= wheelDeltaMin || ev.Momentum {
		return false
	}

	// 已经在最顶部且冷却期过后，再主动向上滚一下才展开搜索栏
	if scrollingUp && atTop && a.controlsCollapsed {
		if a.topReachedAt.IsZero() || ev.Time.Sub(a.topReachedAt) > topSettleDuration {
			a.upScrollAccum += absDY
			if a.upScrollAccum >= searchRevealThreshold {
				a.controlsCollapsed = false
				a.upScrollAccum = 0
				if offset > 0 {
					a.listScroll.Y = 0
				}
			}
			return true
		}
		a.upScrollAccum = 0
		return false
	}

	if !scrollingUp || !atTop {
		a.upScrollAccum = 0
	}
	return false
}

func (a *clipApp) renderCardActionBtn(c *ui.Context, icon *ui.SVG, label string, active bool, onClick func()) {
	t := c.Theme()
	btn := ui.ButtonBase(c).Size(cardActionSize, cardActionSize).Center().Radius(cardActionSize / 2).Label(label)
	bg := t.Surface
	border := t.Border
	fg := t.TextMuted
	if active {
		bg = t.Surface.Mix(t.Accent, 0.12)
		border = t.Accent
		fg = t.Accent
	} else if btn.Hovered() {
		bg = t.SurfaceHover
		border = t.Border.Mix(t.TextMuted, 0.35)
		fg = t.Text
	}
	btn.Background(bg).Border(1, border).Transition(colorTransition).
		OnClick(onClick).
		Children(func() {
			ui.Icon(c, icon).FontSize(12).TextColor(fg)
		})
}

func (a *clipApp) renderImagePreview(c *ui.Context, item store.ClipItem) {
	t := c.Theme()
	var maxW, maxH float32 = 356, 100
	if a.settings.CompactMode {
		maxW, maxH = 120, 38
	}
	if bmp := a.getOrLoadThumb(item.ImagePath); bmp != nil {
		w, h := fitImageSize(bmp, maxW, maxH)
		ui.Image(c, bmp).Size(w, h).Fit(ui.Contain).Radius(radiusControl).
			Border(1, t.Border)
		return
	}
	// 异步加载完成前保持占位高度，防止列表发生跳动
	ui.Box(c).Size(100, maxH).Radius(radiusControl).Background(t.SurfaceHover)
}

func fitImageSize(bmp *ui.Bitmap, maxW, maxH float32) (float32, float32) {
	iw, ih := bmp.Size()
	if iw <= 0 || ih <= 0 {
		return maxH, maxH
	}
	w, h := float32(iw), float32(ih)
	scale := min(maxH/h, maxW/w)
	if scale > 1 {
		scale = 1
	}
	return max(24, w*scale), max(24, h*scale)
}

func (a *clipApp) renderFilesPreview(c *ui.Context, item store.ClipItem) {
	t := c.Theme()
	maxShow := min(3, len(item.Files))
	if a.settings.CompactMode {
		maxShow = min(1, len(item.Files))
	}
	ui.Column(c).Gap(3).Children(func() {
		for i := 0; i < maxShow; i++ {
			p := item.Files[i]
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, filepath.Base(p)).FontSize(14.5).TextColor(t.Text).SingleLine()
				ui.Text(c, filepath.Dir(p)).FontSize(12.5).TextColor(t.TextMuted).SingleLine().Grow(1)
			})
		}
		if len(item.Files) > maxShow {
			ui.Textf(c, "等共 %d 个文件", len(item.Files)).FontSize(12.5).TextColor(t.TextMuted)
		}
	})
}

func (a *clipApp) getOrLoadThumb(path string) *ui.Bitmap {
	return a.thumbs.get(a.panel, path, 120, func() ([]byte, error) {
		return os.ReadFile(path)
	})
}

func (a *clipApp) getOrLoadAppIcon(path string) *ui.Bitmap {
	if path == "" || !mygo.App.IsReady() {
		return nil
	}
	return a.appIcons.get(a.panel, path, 64, func() ([]byte, error) {
		return mygo.Shell.FileIcon(path, 32)
	})
}

func kindMeta(kind store.ClipKind) (*ui.SVG, string) {
	switch kind {
	case store.KindImage:
		return iconImage, "图片"
	case store.KindFile:
		return iconFile, "文件"
	default:
		return iconText, "文本"
	}
}

func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "< 1分钟"
	case d < time.Hour:
		return fmt.Sprintf("%d分钟前", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d小时前", int(d.Hours()))
	default:
		return fmt.Sprintf("%d天前", int(d.Hours()/24))
	}
}
