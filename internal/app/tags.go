package app

import (
	"fmt"
	"log"
	"math"
	"strings"
	"unicode/utf16"

	"mytiez/internal/store"
	"mytiez/locales"

	"github.com/egoist/mygo/ui"
)

func hslToColor(hDeg, s, l float64) ui.Color {
	h := math.Mod(hDeg, 360)
	if h < 0 {
		h += 360
	}
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	clampByte := func(v float64) uint8 {
		n := math.Round((v + m) * 255)
		if n < 0 {
			return 0
		}
		if n > 255 {
			return 255
		}
		return uint8(n)
	}
	return ui.RGB(clampByte(r), clampByte(g), clampByte(b))
}

func tagAccentColor(tag string, dark bool) ui.Color {
	var hash int32
	for _, code := range utf16.Encode([]rune(tag)) {
		hash = int32(code) + ((hash << 5) - hash)
	}
	// 连续色相映射，跳过偏浅/刺眼的黄色区间 [38°, 74°)
	const yellowStart, yellowSpan = 38.0, 36.0
	hue := math.Mod(math.Abs(float64(hash)*137.508+float64(hash>>3)), 360.0-yellowSpan)
	if hue >= yellowStart {
		hue += yellowSpan
	}
	lightness := 0.56
	if dark {
		lightness = 0.60
	}
	return hslToColor(hue, 0.76, lightness)
}

func tagChipStyle(tag string, dark, hovered bool) (bg, fg, border ui.Color, shadowAlpha, shadowBlur, shadowY float32) {
	border = tagAccentColor(tag, dark)
	if dark {
		bg = ui.RGBA(148, 163, 184, 0.18)
		if hovered {
			bg = ui.RGBA(148, 163, 184, 0.28)
			return bg, ui.Hex("#f1f5f9"), border, 0.28, 12, 4
		}
		return bg, ui.Hex("#e2e8f0"), border, 0.20, 8, 2
	}
	bg = ui.Hex("#f1f5f9")
	if hovered {
		bg = ui.Hex("#e2e8f0")
		return bg, ui.Hex("#0f172a"), border, 0.14, 14, 5
	}
	return bg, ui.Hex("#1e293b"), border, 0.08, 8, 2
}

func tagChipAppliedStyle(tag string, dark, hovered bool) (bg, fg, border ui.Color, shadowAlpha, shadowBlur, shadowY float32) {
	border = tagAccentColor(tag, dark)
	if dark {
		base := ui.Hex("#1e293b")
		mixRatio := float32(0.32)
		if hovered {
			mixRatio = 0.42
		}
		bg = base.Mix(border, mixRatio)
		return bg, ui.Hex("#f8fafc"), border, 0.26, 10, 3
	}
	mixRatio := float32(0.20)
	if hovered {
		mixRatio = 0.28
	}
	bg = ui.Hex("#ffffff").Mix(border, mixRatio)
	return bg, ui.Hex("#0f172a"), border, 0.12, 10, 3
}

func (a *clipApp) toggleTagEditor(id int64) {
	if a.editingTagsID == id {
		a.editingTagsID = 0
		a.tagInput = ""
		a.tagFocusPending = false
		a.editingTagOrder = nil
		return
	}
	a.editingTagsID = id
	a.tagInput = ""
	a.tagFocusPending = true
	a.editingTagOrder = append([]string(nil), a.allTags...)
	a.activatePanelForInput()
}

func (a *clipApp) addTagToItem(item store.ClipItem, raw string) {
	if a.store == nil {
		return
	}
	tag := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "#"))
	if tag == "" {
		return
	}
	for _, existing := range item.Tags {
		if existing == tag {
			a.tagInput = ""
			return
		}
	}
	next := append(append([]string{}, item.Tags...), tag)
	if _, err := a.store.UpdateTags(item.ID, next); err != nil {
		log.Printf("update tags: %v", err)
		return
	}
	a.tagInput = ""
	if a.editingTagsID != 0 {
		found := false
		for _, t := range a.editingTagOrder {
			if t == tag {
				found = true
				break
			}
		}
		if !found {
			a.editingTagOrder = append(a.editingTagOrder, tag)
		}
	}
	a.reload()
}

func (a *clipApp) removeTagFromItem(item store.ClipItem, target string) {
	if a.store == nil {
		return
	}
	next := make([]string, 0, len(item.Tags))
	for _, existing := range item.Tags {
		if existing != target {
			next = append(next, existing)
		}
	}
	if _, err := a.store.UpdateTags(item.ID, next); err != nil {
		log.Printf("update tags: %v", err)
		return
	}
	a.reload()
}

func (a *clipApp) filterByTag(tag string) {
	a.stopSearchTimer()
	target := "#" + tag
	if strings.TrimSpace(a.query) == target {
		a.query = ""
	} else {
		a.query = target
	}
	a.selected = -1
	a.resetScrollToTop()
	a.reload()
	a.scrollListToTop()
}

func (a *clipApp) pickableTagSuggestions(item store.ClipItem) []string {
	source := a.allTags
	if len(a.editingTagOrder) > 0 {
		source = a.editingTagOrder
	}
	if len(source) == 0 {
		return nil
	}
	q := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(a.tagInput), "#")))
	var out []string
	for _, t := range source {
		if q != "" && !strings.Contains(strings.ToLower(t), q) {
			continue
		}
		out = append(out, t)
		if len(out) >= 12 {
			break
		}
	}
	return out
}

func (a *clipApp) fitOverlayTags(c *ui.Context, t *ui.Theme, tags []string) (visible []string, hidden int) {
	if len(tags) == 0 {
		return nil, 0
	}
	const (
		maxShow      = 3
		maxChipTextW = float32(88)
		chipPadX     = float32(20)
		gapW         = float32(4)
		moreReserveW = float32(34)
	)
	budget := max(140, (a.panelWidth()-58)*0.55)

	used := float32(0)
	for i, tag := range tags {
		if i >= maxShow {
			break
		}
		tw, _ := c.MeasureText(0, ui.Span{Text: tag, Font: t.Font, Size: 11})
		chipW := min(tw, maxChipTextW) + chipPadX
		need := chipW
		if i > 0 {
			need += gapW
		}
		reserve := float32(0)
		if i+1 < len(tags) {
			reserve = gapW + moreReserveW
		}
		if i > 0 && used+need+reserve > budget {
			break
		}
		used += need
		visible = append(visible, tag)
	}
	return visible, len(tags) - len(visible)
}

func (a *clipApp) renderCardTags(c *ui.Context, t *ui.Theme, item store.ClipItem) {
	editing := a.editingTagsID == item.ID
	if len(item.Tags) == 0 && !editing {
		return
	}

	if !editing {
		visible, hidden := a.fitOverlayTags(c, t, item.Tags)
		bottomInset := float32(8)
		if a.settings.CompactMode {
			bottomInset = 5
		}
		ui.Row(c).Absolute().Right(10).Bottom(bottomInset).AlignItems(ui.Center).Gap(4).Children(func() {
			for _, tag := range visible {
				tagName := tag
				chip := ui.ButtonBase(c).Padding(3, 10).Radius(radiusPill).Label("#" + tagName)
				chipBg, chipFg, chipBorder, sAlpha, sBlur, sY := tagChipStyle(tagName, t.Dark, chip.Hovered())
				chip.Background(chipBg).Border(1.25, chipBorder).
					Shadow(0, sY, sBlur, 0, ui.RGBA(15, 23, 42, sAlpha)).
					Transition(colorTransition).
					OnClick(func() {
						a.filterByTag(tagName)
					}).
					Children(func() {
						ui.Text(c, tagName).MaxWidth(88).FontSize(10.5).FontWeight(600).TextColor(chipFg).SingleLine()
					})
			}
			if hidden > 0 {
				msgs := locales.In(c.Locale())
				moreText := fmt.Sprintf("+%d", hidden)
				moreBg := t.SurfaceHover
				moreBtn := ui.ButtonBase(c).Padding(3, 8).Radius(radiusPill).Label(msgs.TagsExpandAll())
				if moreBtn.Hovered() {
					moreBg = t.SurfacePressed
				}
				moreBtn.Background(moreBg).Border(1, t.Border).
					Shadow(0, 2, 6, 0, ui.RGBA(15, 23, 42, 0.06)).
					OnClick(func() {
						a.toggleTagEditor(item.ID)
					}).
					Children(func() {
						ui.Text(c, moreText).FontSize(10.5).FontWeight(600).TextColor(t.TextMuted).SingleLine()
					})
			}
		})
		return
	}

	msgs := locales.In(c.Locale())
	existingTags := make(map[string]struct{}, len(item.Tags))
	for _, tag := range item.Tags {
		existingTags[tag] = struct{}{}
	}
	suggestions := a.pickableTagSuggestions(item)
	suggestionSet := make(map[string]struct{}, len(suggestions))
	for _, sug := range suggestions {
		suggestionSet[sug] = struct{}{}
	}
	var extraApplied []string
	for _, tag := range item.Tags {
		if _, shownBelow := suggestionSet[tag]; !shownBelow {
			extraApplied = append(extraApplied, tag)
		}
	}

	ui.Column(c).FillWidth().Padding(5, 0, 0, 0).Gap(6).Children(func() {
		if len(extraApplied) > 0 {
			ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(5).Children(func() {
				for _, tag := range extraApplied {
					tagName := tag
					chip := ui.ButtonBase(c).Padding(3, 10).Radius(radiusPill).Label(msgs.TagsRemoveA11y(tagName))
					chipBg, chipFg, chipBorder, sAlpha, sBlur, sY := tagChipStyle(tagName, t.Dark, chip.Hovered())
					chip.Background(chipBg).Border(1.25, chipBorder).
						Shadow(0, sY, sBlur, 0, ui.RGBA(15, 23, 42, sAlpha)).
						Transition(colorTransition).
						OnClick(func() {
							a.removeTagFromItem(item, tagName)
						}).
						Children(func() {
							ui.Row(c).AlignItems(ui.Center).Gap(4).Children(func() {
								ui.Text(c, "×").FontSize(10.5).FontWeight(700).TextColor(chipFg.Alpha(0.75)).SingleLine()
								ui.Text(c, tagName).FontSize(10.5).FontWeight(600).TextColor(chipFg).SingleLine()
							})
						})
				}
			})
		}

		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).
			ObserveInput(func(ev ui.InputEvent) {
				if ev.Kind == ui.InputPointerDown {
					a.activatePanelForInput()
				}
			}).
			Children(func() {
			inp := ui.TextInput(c, &a.tagInput).Bind(&a.tagInputHandle).
				Placeholder(msgs.TagsInputPlaceholder()).
				Grow(1).Height(26).Padding(0, 8).Radius(radiusMicro).
				FontSize(12).Background(t.Background).Border(1, t.Border).Label(msgs.TagsInputLabel())
			if a.tagFocusPending {
				a.tagFocusPending = false
				a.tagInputHandle.Focus(c)
			}
			if inp.Submitted() {
				a.addTagToItem(item, a.tagInput)
			}

			addBtn := ui.ButtonBase(c).Height(26).Padding(0, 9).Radius(radiusMicro).Label(msgs.TagsAddA11y())
			addBg := t.Accent
			if addBtn.Hovered() {
				addBg = t.AccentHover
			}
			addBtn.Background(addBg).
				OnClick(func() {
					a.addTagToItem(item, a.tagInput)
				}).
				Children(func() {
					ui.Text(c, msgs.TagsAddBtn()).FontSize(11.5).FontWeight(600).TextColor(t.AccentText)
				})

			doneBtn := ui.ButtonBase(c).Height(26).Padding(0, 9).Radius(radiusMicro).Label(msgs.TagsDoneA11y())
			doneBg := t.Surface
			if doneBtn.Hovered() {
				doneBg = t.SurfaceHover
			}
			doneBtn.Background(doneBg).Border(1, t.Border).
				OnClick(func() {
					a.editingTagsID = 0
					a.tagInput = ""
					a.editingTagOrder = nil
				}).
				Children(func() {
					ui.Text(c, msgs.TagsDoneBtn()).FontSize(11.5).FontWeight(600).TextColor(t.TextMuted)
				})
		})

		if len(suggestions) > 0 {
			ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(5).Children(func() {
				for _, sTag := range suggestions {
					sug := sTag
					_, applied := existingTags[sug]
					a11y := msgs.TagsSelectA11y(sug)
					prefix := "+"
					if applied {
						a11y = msgs.TagsRemoveA11y(sug)
						prefix = "×"
					}
					btn := ui.ButtonBase(c).Padding(3, 10).Radius(radiusPill).Label(a11y)
					var sBg, sFg, sBorder ui.Color
					var sAlpha, sBlur, sY float32
					borderWidth := float32(1)
					fontWeight := 600
					prefixColor := ui.Color{}
					if applied {
						sBg, sFg, sBorder, sAlpha, sBlur, sY = tagChipAppliedStyle(sug, t.Dark, btn.Hovered())
						borderWidth = 1.75
						fontWeight = 700
						prefixColor = sBorder
					} else {
						sBg, sFg, sBorder, sAlpha, sBlur, sY = tagChipStyle(sug, t.Dark, btn.Hovered())
						sBorder = sBorder.Alpha(0.55)
						prefixColor = sFg.Alpha(0.75)
					}
					btn.Background(sBg).Border(borderWidth, sBorder).
						Shadow(0, sY, sBlur, 0, ui.RGBA(15, 23, 42, sAlpha)).
						Transition(colorTransition).
						OnClick(func() {
							if applied {
								a.removeTagFromItem(item, sug)
							} else {
								a.addTagToItem(item, sug)
							}
						}).
						Children(func() {
							ui.Row(c).AlignItems(ui.Center).Gap(4).Children(func() {
								ui.Text(c, prefix).FontSize(10.5).FontWeight(700).TextColor(prefixColor).SingleLine()
								ui.Text(c, sug).FontSize(10.5).FontWeight(fontWeight).TextColor(sFg).SingleLine()
							})
						})
				}
			})
		}
	})
}

func (a *clipApp) renderSearchTagPopover(c *ui.Context, t *ui.Theme) {
	if !a.searchHandle.Focused(c) || a.controlsCollapsed || strings.TrimSpace(a.query) != "" || len(a.allTags) == 0 {
		return
	}
	tags := a.allTags
	if len(tags) > 18 {
		tags = tags[:18]
	}

	popBg := ui.RGBA(255, 255, 255, 0.42)
	shadowAlpha := float32(0.08)
	if t.Dark {
		popBg = ui.RGBA(36, 36, 36, 0.48)
		shadowAlpha = 0.24
	}

	msgs := locales.In(c.Locale())
	ui.Column(c).Absolute().Left(16).Right(16).Top(90).ZIndex(30).KeepFocus().
		Padding(10, 12, 12, 12).Gap(8).Radius(16).
		Background(popBg).Border(1, t.Border.Alpha(0.55)).
		Shadow(0, 8, 20, 0, ui.RGBA(0, 0, 0, shadowAlpha)).
		Children(func() {
			ui.Text(c, msgs.TagsPopoverTitle()).FontSize(11).FontWeight(700).TextColor(t.TextMuted.Alpha(0.85))
			ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(6).Children(func() {
				for _, tag := range tags {
					tagName := tag
					btn := ui.ButtonBase(c).Padding(4, 10).Radius(radiusPill).Label(msgs.TagsFilterA11y(tagName))
					chipBg, chipFg, chipBorder, sAlpha, sBlur, sY := tagChipStyle(tagName, t.Dark, btn.Hovered())
					btn.Background(chipBg).Border(1.25, chipBorder).
						Shadow(0, sY, sBlur, 0, ui.RGBA(15, 23, 42, sAlpha)).
						Transition(colorTransition).
						OnClick(func() {
							a.filterByTag(tagName)
						}).
						Children(func() {
							ui.Text(c, tagName).MaxWidth(110).FontSize(11).FontWeight(600).TextColor(chipFg).SingleLine()
						})
				}
			})
		})
}
