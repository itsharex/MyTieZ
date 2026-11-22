package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"mytiez/internal/store"
	"mytiez/locales"

	"github.com/egoist/mygo/ui"
)

const (
	maxFileMenuEntries    = 50
	maxFileMenuDirEntries = 5
	fileMenuStatTTL       = time.Second

	cardMenuMinWidth      float32 = 104
	cardMenuMaxWidth      float32 = 124
	fileMenuMultiMaxWidth float32 = 236
	fileMenuTitleFontSize  float32 = 12
	fileMenuSubFontSize    float32 = 10.5
	fileMenuPanelRadius    float32 = 10
)

type cardMenuAction struct {
	id              string
	separatorBefore bool
	visible         func(a *clipApp, item store.ClipItem) bool
	build           func(a *clipApp, c *ui.Context, msgs locales.Messages, item store.ClipItem, m *ui.Menu)
}

// cardContextMenuActions centralizes every item shown in the card context menu
// in display order so future actions only need a single entry here.
var cardContextMenuActions = []cardMenuAction{
	{
		id: "open",
		visible: func(_ *clipApp, item store.ClipItem) bool {
			return item.Kind != store.KindFile || len(nonEmptyPaths(item.Files)) > 0
		},
		build: func(a *clipApp, c *ui.Context, msgs locales.Messages, item store.ClipItem, m *ui.Menu) {
			if item.Kind != store.KindFile {
				title := msgs.CardActionOpen()
				if styleCardMenuItem(m.Item(title).ID("card-menu:open"), title, "").Chosen() {
					a.openItem(item)
				}
				return
			}
			files := nonEmptyPaths(item.Files)
			if len(files) == 1 {
				a.buildSingleFileMenuItems(c, item, m)
				return
			}
			openTitle := msgs.CardActionOpen()
			subIt := m.Submenu(openTitle, func(sub *ui.Menu) {
				a.buildMultiFileMenu(c, item, sub)
			}).ID("card-menu:open-files")
			styleCardMenuItem(subIt, openTitle, "")
		},
	},
	{
		id: "plain_paste",
		visible: func(_ *clipApp, item store.ClipItem) bool {
			return item.Kind == store.KindRichText
		},
		build: func(a *clipApp, _ *ui.Context, msgs locales.Messages, item store.ClipItem, m *ui.Menu) {
			plainLabel := msgs.RichMenuPastePlain()
			if a.settings.ClickCopyOnly {
				plainLabel = msgs.RichMenuCopyPlain()
			}
			plainIt := m.Item(plainLabel).ID("card-menu:plain-paste")
			if styleCardMenuItem(plainIt, plainLabel, "").Chosen() {
				a.pasteItem(item, true)
			}
		},
	},
	{
		id: "edit_tags",
		visible: func(_ *clipApp, _ store.ClipItem) bool {
			return true
		},
		build: func(a *clipApp, _ *ui.Context, msgs locales.Messages, item store.ClipItem, m *ui.Menu) {
			title := msgs.CardMenuEditTags()
			if styleCardMenuItem(m.Item(title).ID("card-menu:edit-tags"), title, "").Chosen() {
				a.openTagEditor(item.ID)
			}
		},
	},
	{
		id: "quick_tag",
		visible: func(a *clipApp, _ store.ClipItem) bool {
			return len(a.allTags) > 0
		},
		build: func(a *clipApp, _ *ui.Context, msgs locales.Messages, item store.ClipItem, m *ui.Menu) {
			subTitle := msgs.CardMenuQuickTag()
			subIt := m.Submenu(subTitle, func(sub *ui.Menu) {
				sub.MaxWidth(min(cardMenuMaxWidth, a.panelWidth()-64))
				existing := make(map[string]struct{}, len(item.Tags))
				for _, tg := range item.Tags {
					existing[tg] = struct{}{}
				}
				for i, tag := range a.allTags {
					tagName := tag
					_, applied := existing[tagName]
					it := sub.Item(tagName).
						ID(fmt.Sprintf("card-menu:quick-tag:%d:%s", i, tagName)).
						Tooltip(tagName).
						Disabled(applied)
					if styleCardMenuItem(it, tagName, "").Chosen() {
						a.addTagToItem(item, tagName)
					}
				}
			}).ID("card-menu:quick-tag")
			styleCardMenuItem(subIt, subTitle, "")
		},
	},
	{
		id: "pin",
		visible: func(_ *clipApp, _ store.ClipItem) bool {
			return true
		},
		build: func(a *clipApp, _ *ui.Context, msgs locales.Messages, item store.ClipItem, m *ui.Menu) {
			title := msgs.CardActionPin()
			if item.Pinned {
				title = msgs.CardMenuUnpin()
			}
			if styleCardMenuItem(m.Item(title).ID("card-menu:pin"), title, "").Chosen() {
				a.togglePin(item.ID)
			}
		},
	},
	{
		id:              "delete",
		separatorBefore: true,
		visible: func(_ *clipApp, _ store.ClipItem) bool {
			return true
		},
		build: func(a *clipApp, _ *ui.Context, msgs locales.Messages, item store.ClipItem, m *ui.Menu) {
			title := msgs.CardActionDelete()
			if styleCardMenuItem(m.Item(title).ID("card-menu:delete").Danger(), title, "").Chosen() {
				a.deleteItem(item.ID)
			}
		},
	},
}

func (a *clipApp) fileMenuOpenPtr(id int64) *bool {
	if a.fileMenuOpen == nil {
		a.fileMenuOpen = make(map[int64]*bool)
	}
	if ptr, ok := a.fileMenuOpen[id]; ok {
		return ptr
	}
	if len(a.fileMenuOpen) > 64 && !a.hasOpenFileMenu() {
		a.fileMenuOpen = make(map[int64]*bool)
	}
	open := false
	a.fileMenuOpen[id] = &open
	return &open
}

func (a *clipApp) isFileMenuOpen(id int64) bool {
	if a.fileMenuOpen == nil {
		return false
	}
	ptr, ok := a.fileMenuOpen[id]
	return ok && ptr != nil && *ptr
}

func (a *clipApp) hasOpenFileMenu() bool {
	for _, ptr := range a.fileMenuOpen {
		if ptr != nil && *ptr {
			return true
		}
	}
	return false
}

func (a *clipApp) closeFileMenus() {
	for _, ptr := range a.fileMenuOpen {
		if ptr != nil {
			*ptr = false
		}
	}
	a.fileMenuOpen = nil
	a.fileMenuStatID = 0
	a.fileMenuStatAt = time.Time{}
	a.fileMenuStatPaths = nil
	a.fileMenuExists = nil
}

func (a *clipApp) fileMenuExistMap(item store.ClipItem, files []string) map[string]bool {
	if a.fileMenuStatID == item.ID && a.fileMenuExists != nil &&
		time.Since(a.fileMenuStatAt) < fileMenuStatTTL &&
		slices.Equal(a.fileMenuStatPaths, files) {
		return a.fileMenuExists
	}

	exists := make(map[string]bool, min(len(files), maxFileMenuEntries)+1)
	limit := min(len(files), maxFileMenuEntries)
	anyFound := false
	for i := 0; i < limit; i++ {
		p := files[i]
		_, err := os.Stat(p)
		ok := err == nil
		exists[p] = ok
		if ok {
			anyFound = true
		}
	}
	if !anyFound && len(files) > limit {
		for i := limit; i < len(files); i++ {
			p := files[i]
			if _, err := os.Stat(p); err == nil {
				exists[p] = true
				break
			}
		}
	}

	a.fileMenuStatID = item.ID
	a.fileMenuStatAt = time.Now()
	a.fileMenuStatPaths = slices.Clone(files)
	a.fileMenuExists = exists
	return exists
}

func fileDisplayName(p string) string {
	base := filepath.Base(p)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return p
	}
	return base
}

func sameFileDir(a, b string) bool {
	ca, cb := filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(ca, cb)
	}
	return ca == cb
}

func collectExistingDirs(files []string, exists map[string]bool) (firstExist string, distinctDirFiles []string) {
	seenDirs := make([]string, 0, 8)
	for _, p := range files {
		if !exists[p] {
			continue
		}
		if firstExist == "" {
			firstExist = p
		}
		dir := filepath.Dir(p)
		seen := false
		for _, prev := range seenDirs {
			if sameFileDir(prev, dir) {
				seen = true
				break
			}
		}
		if !seen && len(distinctDirFiles) < maxFileMenuEntries {
			seenDirs = append(seenDirs, dir)
			distinctDirFiles = append(distinctDirFiles, p)
		}
	}
	return firstExist, distinctDirFiles
}

func hasDuplicateBaseNames(files []string) bool {
	seen := make(map[string]struct{}, len(files))
	for _, p := range files {
		name := fileDisplayName(p)
		if runtime.GOOS == "windows" {
			name = strings.ToLower(name)
		}
		if _, ok := seen[name]; ok {
			return true
		}
		seen[name] = struct{}{}
	}
	return false
}

// cardPopoverPanelStyle keeps menu panels themed and bounded so submenus fit
// comfortably inside the 400px panel even when flipped horizontally or opened
// near the center.
func (a *clipApp) cardPopoverPanelStyle(c *ui.Context, t *ui.Theme, maxW float32) func(panel ui.Element) {
	bg := a.floatingSurface(c, t)
	sa := shadowAlpha(t, 0.14, 0.36)
	panelIdx := 0
	return func(panel ui.Element) {
		minW, curMaxW := cardMenuMinWidth, maxW
		if panelIdx == 0 {
			panel.Bind(&a.cardMenuRootHandle)
		} else if rb := a.cardMenuRootHandle.Bounds(c); rb.W > 0 {
			const edgeMargin float32 = 4
			leftRoom := rb.X - edgeMargin
			rightRoom := a.panelWidth() - edgeMargin - (rb.X + rb.W)
			if availW := max(leftRoom, rightRoom); availW > 0 {
				curMaxW = max(72, min(maxW, availW))
				minW = min(cardMenuMinWidth, curMaxW)
			}
		}
		panelIdx++
		panel.MinWidth(minW).MaxWidth(curMaxW).
			Padding(4).Radius(fileMenuPanelRadius).
			Background(bg).Border(1, t.Border).
			NoShadow().Shadow(0, 6, 18, 0, ui.RGBA(0, 0, 0, sa))
	}
}

func cardPopoverItemStyle(item ui.Element) {
	item.Padding(4, 8).Radius(radiusMicro)
}

func styleCardMenuItem(it *ui.MenuItem, title, subtitle string) *ui.MenuItem {
	if subtitle != "" {
		it.Subtitle(subtitle)
	}
	return it.Content(func(mc ui.MenuItemContent) {
		ui.Text(mc.Context, title).
			FontSize(fileMenuTitleFontSize).
			TextColor(mc.Text).
			SingleLine()
		if subtitle != "" {
			subColor := mc.Muted
			if mc.Highlighted {
				subColor = subColor.Alpha(0.78)
			}
			ui.Text(mc.Context, subtitle).
				FontSize(fileMenuSubFontSize).
				TextColor(subColor).
				Margin(1, 0, 0, 0).
				SingleLine()
		}
	})
}

func (a *clipApp) buildMultiFileMenu(c *ui.Context, item store.ClipItem, m *ui.Menu) {
	files := nonEmptyPaths(item.Files)
	if len(files) == 0 {
		return
	}
	msgs := locales.In(c.Locale())
	m.MaxWidth(min(fileMenuMultiMaxWidth, a.panelWidth()-64))

	exists := a.fileMenuExistMap(item, files)
	firstExist, distinctDirFiles := collectExistingDirs(files, exists)

	switch {
	case firstExist == "":
		title := msgs.FileMenuShowInFolder()
		styleCardMenuItem(m.Item(title).ID("file-menu:show-folder").Disabled(true), title, "")
	case len(distinctDirFiles) <= 1:
		dir := filepath.Dir(firstExist)
		title := msgs.FileMenuShowInFolder()
		if styleCardMenuItem(m.Item(title).ID("file-menu:show-folder").Tooltip(dir), title, "").Chosen() {
			a.showFileInFolder(firstExist)
		}
	default:
		dirLimit := min(len(distinctDirFiles), maxFileMenuDirEntries)
		for idx := 0; idx < dirLimit; idx++ {
			rep := distinctDirFiles[idx]
			dir := filepath.Dir(rep)
			title := msgs.FileMenuShowNamedFolder(fileDisplayName(dir))
			if styleCardMenuItem(
				m.Item(title).ID(fmt.Sprintf("file-menu:dir:%d:%s", idx, rep)).Tooltip(dir),
				title, dir,
			).Chosen() {
				a.showFileInFolder(rep)
			}
		}
	}

	m.Separator()

	limit := min(len(files), maxFileMenuEntries)
	showDirSubtitle := len(distinctDirFiles) > 1 || hasDuplicateBaseNames(files[:limit])
	for i := 0; i < limit; i++ {
		p := files[i]
		name := fileDisplayName(p)
		it := m.Item(name).ID(fmt.Sprintf("file-menu:open:%d:%s", i, p)).Tooltip(p)
		sub := ""
		if !exists[p] {
			it.Disabled(true)
			sub = msgs.FileMenuMissing()
		} else if showDirSubtitle {
			sub = filepath.Dir(p)
		}
		if styleCardMenuItem(it, name, sub).Chosen() {
			a.openFilePath(p)
		}
	}

	if len(files) > limit {
		m.Separator()
		more := msgs.FileMenuMoreOmitted(len(files) - limit)
		styleCardMenuItem(m.Item(more).ID("file-menu:more").Disabled(true), more, "")
	}
}

func (a *clipApp) buildSingleFileMenuItems(c *ui.Context, item store.ClipItem, m *ui.Menu) {
	files := nonEmptyPaths(item.Files)
	if len(files) == 0 {
		return
	}
	msgs := locales.In(c.Locale())

	p := files[0]
	exists := a.fileMenuExistMap(item, files)[p]

	openTitle := msgs.CardActionOpen()
	openIt := m.Item(openTitle).ID("file-menu:single-open").Tooltip(p).Disabled(!exists)
	openSub := ""
	if !exists {
		openSub = msgs.FileMenuMissing()
	}
	if styleCardMenuItem(openIt, openTitle, openSub).Chosen() {
		a.openFilePath(p)
	}

	dir := filepath.Dir(p)
	showTitle := msgs.FileMenuShowInFolder()
	showIt := styleCardMenuItem(
		m.Item(showTitle).ID("file-menu:single-folder").Tooltip(dir).Disabled(!exists),
		showTitle, "",
	)
	if showIt.Chosen() {
		a.showFileInFolder(p)
	}
}

func (a *clipApp) renderFileOpenMenuBtn(c *ui.Context, item store.ClipItem, active bool) {
	t := c.Theme()
	msgs := locales.In(c.Locale())
	label := msgs.CardActionOpenFiles()
	btn := ui.ButtonBase(c.Key("file-open-menu")).
		Size(cardActionSize, cardActionSize).Center().Radius(cardActionSize / 2).
		Label(label).Tooltip(label)
	bg := t.Surface
	border := t.Border
	fg := t.Text.Mix(t.TextMuted, 0.55)
	if active {
		bg = t.Accent
		border = t.Accent
		fg = t.AccentText
	} else if btn.Hovered() {
		bg = t.SurfaceHover
		border = t.Border.Mix(t.TextMuted, 0.35)
		fg = t.Text
	}
	btn.Background(bg).Border(1, border).Transition(colorTransition).
		Children(func() {
			ui.Icon(c, iconOpen).FontSize(13).TextColor(fg)
		}).
		PopoverMenuButton(func(m *ui.Menu) {
			a.buildMultiFileMenu(c, item, m)
		}, ui.PopoverMenuButtonOptions{
			At:    ui.AnchorBottomEnd,
			Self:  ui.AnchorTopEnd,
			Open:  a.fileMenuOpenPtr(item.ID),
			Panel: a.cardPopoverPanelStyle(c, t, min(fileMenuMultiMaxWidth, a.panelWidth()-64)),
			Item:  cardPopoverItemStyle,
		})
}

func (a *clipApp) buildCardContextMenu(c *ui.Context, item store.ClipItem, m *ui.Menu) {
	msgs := locales.In(c.Locale())
	m.MaxWidth(min(cardMenuMaxWidth, a.panelWidth()-64))

	hasPrev := false
	for _, act := range cardContextMenuActions {
		if act.visible != nil && !act.visible(a, item) {
			continue
		}
		if act.separatorBefore && hasPrev {
			m.Separator()
		}
		act.build(a, c, msgs, item, m)
		hasPrev = true
	}
}

func (a *clipApp) attachCardContextMenu(c *ui.Context, card ui.Element, item store.ClipItem) {
	t := c.Theme()
	card.PopoverMenu(func(m *ui.Menu) {
		a.buildCardContextMenu(c, item, m)
	}, ui.PopoverMenuOptions{
		Panel: a.cardPopoverPanelStyle(c, t, min(cardMenuMaxWidth, a.panelWidth()-64)),
		Item:  cardPopoverItemStyle,
	})
}
