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

	fileMenuMinWidth        float32 = 156
	fileMenuMultiMaxWidth   float32 = 236
	fileMenuSingleMaxWidth  float32 = 204
	fileMenuTitleFontSize   float32 = 12
	fileMenuSubFontSize     float32 = 10.5
	fileMenuPanelRadius     float32 = 10
)

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

func (a *clipApp) filePopoverPanelStyle(c *ui.Context, t *ui.Theme, maxW float32) func(panel ui.Element) {
	bg := a.floatingSurface(c, t)
	sa := shadowAlpha(t, 0.14, 0.36)
	return func(panel ui.Element) {
		panel.MinWidth(fileMenuMinWidth).MaxWidth(maxW).
			Padding(4).Radius(fileMenuPanelRadius).
			Background(bg).Border(1, t.Border).
			NoShadow().Shadow(0, 6, 18, 0, ui.RGBA(0, 0, 0, sa))
	}
}

func filePopoverItemStyle(item ui.Element) {
	item.Padding(4, 8).Radius(radiusMicro)
}

func styleFileMenuItem(it *ui.MenuItem, title, subtitle string) *ui.MenuItem {
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
		styleFileMenuItem(m.Item(title).ID("file-menu:show-folder").Disabled(true), title, "")
	case len(distinctDirFiles) <= 1:
		dir := filepath.Dir(firstExist)
		title := msgs.FileMenuShowInFolder()
		if styleFileMenuItem(m.Item(title).ID("file-menu:show-folder").Tooltip(dir), title, "").Chosen() {
			a.showFileInFolder(firstExist)
		}
	default:
		dirLimit := min(len(distinctDirFiles), maxFileMenuDirEntries)
		for idx := 0; idx < dirLimit; idx++ {
			rep := distinctDirFiles[idx]
			dir := filepath.Dir(rep)
			title := msgs.FileMenuShowNamedFolder(fileDisplayName(dir))
			if styleFileMenuItem(
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
		if styleFileMenuItem(it, name, sub).Chosen() {
			a.openFilePath(p)
		}
	}

	if len(files) > limit {
		m.Separator()
		more := msgs.FileMenuMoreOmitted(len(files) - limit)
		styleFileMenuItem(m.Item(more).ID("file-menu:more").Disabled(true), more, "")
	}
}

func (a *clipApp) buildSingleFileMenu(c *ui.Context, item store.ClipItem, m *ui.Menu) {
	files := nonEmptyPaths(item.Files)
	if len(files) == 0 {
		return
	}
	msgs := locales.In(c.Locale())
	m.MaxWidth(min(fileMenuSingleMaxWidth, a.panelWidth()-64))

	p := files[0]
	exists := a.fileMenuExistMap(item, files)[p]

	openTitle := msgs.CardActionOpen()
	openIt := m.Item(openTitle).ID("file-menu:single-open").Tooltip(p).Disabled(!exists)
	openSub := ""
	if !exists {
		openSub = msgs.FileMenuMissing()
	}
	if styleFileMenuItem(openIt, openTitle, openSub).Chosen() {
		a.openFilePath(p)
	}

	dir := filepath.Dir(p)
	showTitle := msgs.FileMenuShowInFolder()
	showIt := styleFileMenuItem(
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
			Panel: a.filePopoverPanelStyle(c, t, min(fileMenuMultiMaxWidth, a.panelWidth()-64)),
			Item:  filePopoverItemStyle,
		})
}

func (a *clipApp) attachFileCardContextMenu(c *ui.Context, card ui.Element, item store.ClipItem) {
	if item.Kind != store.KindFile {
		return
	}
	files := nonEmptyPaths(item.Files)
	if len(files) == 0 {
		return
	}
	t := c.Theme()
	maxW := min(fileMenuMultiMaxWidth, a.panelWidth()-64)
	if len(files) == 1 {
		maxW = min(fileMenuSingleMaxWidth, a.panelWidth()-64)
	}
	card.PopoverMenu(func(m *ui.Menu) {
		if len(files) == 1 {
			a.buildSingleFileMenu(c, item, m)
			return
		}
		a.buildMultiFileMenu(c, item, m)
	}, ui.PopoverMenuOptions{
		Panel: a.filePopoverPanelStyle(c, t, maxW),
		Item:  filePopoverItemStyle,
	})
}
