package app

import (
	"fmt"
	"hash/fnv"
	"log"
	"strings"

	"mytiez/internal/store"

	"github.com/egoist/mygo/ui"
)

type tagPalette struct {
	lightBg, lightFg ui.Color
	darkBg, darkFg   ui.Color
}

var tagPalettes = []tagPalette{
	{ui.Hex("#e8f0fe"), ui.Hex("#1d4ed8"), ui.RGBA(59, 130, 246, 0.22), ui.Hex("#93c5fd")},
	{ui.Hex("#ecfdf3"), ui.Hex("#047857"), ui.RGBA(16, 185, 129, 0.22), ui.Hex("#6ee7b7")},
	{ui.Hex("#fef3c7"), ui.Hex("#b45309"), ui.RGBA(245, 158, 11, 0.22), ui.Hex("#fcd34d")},
	{ui.Hex("#fce7f3"), ui.Hex("#be185d"), ui.RGBA(236, 72, 153, 0.22), ui.Hex("#f9a8d4")},
	{ui.Hex("#f3e8ff"), ui.Hex("#7e22ce"), ui.RGBA(168, 85, 247, 0.22), ui.Hex("#d8b4fe")},
	{ui.Hex("#e0f2fe"), ui.Hex("#0369a1"), ui.RGBA(14, 165, 233, 0.22), ui.Hex("#7dd3fc")},
	{ui.Hex("#ffedd5"), ui.Hex("#c2410c"), ui.RGBA(249, 115, 22, 0.22), ui.Hex("#fdba74")},
	{ui.Hex("#f1f5f9"), ui.Hex("#334155"), ui.RGBA(148, 163, 184, 0.22), ui.Hex("#cbd5e1")},
}

func tagChipColors(tag string, dark bool) (bg, fg ui.Color) {
	h := fnv.New32a()
	_, _ = h.Write([]byte(tag))
	p := tagPalettes[int(h.Sum32())%len(tagPalettes)]
	if dark {
		return p.darkBg, p.darkFg
	}
	return p.lightBg, p.lightFg
}

func (a *clipApp) toggleTagEditor(id int64) {
	if a.editingTagsID == id {
		a.editingTagsID = 0
		a.tagInput = ""
		a.tagFocusPending = false
		return
	}
	a.editingTagsID = id
	a.tagInput = ""
	a.tagFocusPending = true
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
	target := "#" + tag
	if strings.TrimSpace(a.query) == target {
		a.query = ""
	} else {
		a.query = target
	}
	a.reload()
}

func (a *clipApp) pickableTagSuggestions(item store.ClipItem) []string {
	if len(a.allTags) == 0 {
		return nil
	}
	existing := make(map[string]struct{}, len(item.Tags))
	for _, t := range item.Tags {
		existing[t] = struct{}{}
	}
	q := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(a.tagInput), "#")))
	var out []string
	for _, t := range a.allTags {
		if _, ok := existing[t]; ok {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(t), q) {
			continue
		}
		out = append(out, t)
		if len(out) >= 6 {
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
		chipPadX     = float32(14)
		gapW         = float32(4)
		moreReserveW = float32(34)
	)
	winW := float32(a.settings.WindowWidth)
	if a.panel != nil {
		if w, _ := a.panel.Size(); w > 0 {
			winW = float32(w)
		}
	}
	if winW <= 0 {
		winW = panelWidth
	}
	budget := max(140, (winW-58)*0.55)

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
				chipBg, chipFg := tagChipColors(tagName, t.Dark)
				chip := ui.ButtonBase(c).Padding(2, 7).Radius(radiusMicro).Label("#"+tagName).
					Background(chipBg).Border(1, t.Surface.Alpha(0.85)).
					OnClick(func() {
						a.filterByTag(tagName)
					})
				chip.Children(func() {
					ui.Text(c, tagName).MaxWidth(88).FontSize(11).FontWeight(600).TextColor(chipFg).SingleLine()
				})
			}
			if hidden > 0 {
				moreText := fmt.Sprintf("+%d", hidden)
				moreBg := t.SurfaceHover
				moreBtn := ui.ButtonBase(c).Padding(2, 6).Radius(radiusMicro).Label("展开全部标签")
				if moreBtn.Hovered() {
					moreBg = t.SurfacePressed
				}
				moreBtn.Background(moreBg).Border(1, t.Border).
					OnClick(func() {
						a.toggleTagEditor(item.ID)
					}).
					Children(func() {
						ui.Text(c, moreText).FontSize(11).FontWeight(600).TextColor(t.TextMuted).SingleLine()
					})
			}
		})
		return
	}

	ui.Column(c).FillWidth().Padding(5, 0, 0, 0).Gap(6).Children(func() {
		if len(item.Tags) > 0 {
			ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(5).Children(func() {
				for _, tag := range item.Tags {
					tagName := tag
					chipBg, chipFg := tagChipColors(tagName, t.Dark)
					chip := ui.ButtonBase(c).Padding(2, 7).Radius(radiusMicro).Label("移除标签 " + tagName).
						Background(chipBg).
						OnClick(func() {
							a.removeTagFromItem(item, tagName)
						})
					chip.Children(func() {
						ui.Row(c).AlignItems(ui.Center).Gap(4).Children(func() {
							ui.Text(c, tagName).FontSize(11).FontWeight(600).TextColor(chipFg).SingleLine()
							ui.Text(c, "×").FontSize(11).FontWeight(700).TextColor(chipFg.Alpha(0.78)).SingleLine()
						})
					})
				}
			})
		}

		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
			inp := ui.TextInput(c, &a.tagInput).Bind(&a.tagInputHandle).
				Placeholder("输入标签回车添加...").
				Grow(1).Height(26).Padding(0, 8).Radius(radiusMicro).
				FontSize(12).Background(t.Background).Border(1, t.Border).Label("标签输入框")
			if a.tagFocusPending {
				a.tagFocusPending = false
				a.tagInputHandle.Focus(c)
			}
			if inp.Submitted() {
				a.addTagToItem(item, a.tagInput)
			}

			addBtn := ui.ButtonBase(c).Height(26).Padding(0, 9).Radius(radiusMicro).Label("添加标签")
			addBg := t.Accent
			if addBtn.Hovered() {
				addBg = t.AccentHover
			}
			addBtn.Background(addBg).
				OnClick(func() {
					a.addTagToItem(item, a.tagInput)
				}).
				Children(func() {
					ui.Text(c, "添加").FontSize(11.5).FontWeight(600).TextColor(t.AccentText)
				})

			doneBtn := ui.ButtonBase(c).Height(26).Padding(0, 9).Radius(radiusMicro).Label("完成标签编辑")
			doneBg := t.Surface
			if doneBtn.Hovered() {
				doneBg = t.SurfaceHover
			}
			doneBtn.Background(doneBg).Border(1, t.Border).
				OnClick(func() {
					a.editingTagsID = 0
					a.tagInput = ""
				}).
				Children(func() {
					ui.Text(c, "完成").FontSize(11.5).FontWeight(600).TextColor(t.TextMuted)
				})
		})

		if suggestions := a.pickableTagSuggestions(item); len(suggestions) > 0 {
			ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(5).Children(func() {
				for _, sTag := range suggestions {
					sug := sTag
					sBg, sFg := tagChipColors(sug, t.Dark)
					btn := ui.ButtonBase(c).Padding(2, 7).Radius(radiusMicro).Label("选择标签 " + sug).
						Background(sBg.Alpha(0.75)).
						OnClick(func() {
							a.addTagToItem(item, sug)
						})
					btn.Children(func() {
						ui.Text(c, "+ "+sug).FontSize(11).FontWeight(500).TextColor(sFg).SingleLine()
					})
				}
			})
		}
	})
}
