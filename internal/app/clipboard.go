package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"os"
	"slices"
	"strings"
	"time"

	"mytiez/internal/store"
	"mytiez/internal/sys"
	"mytiez/locales"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

func (a *clipApp) currentKind() store.ClipKind {
	switch a.filterIdx {
	case 1:
		return store.KindText
	case 2:
		return store.KindImage
	case 3:
		return store.KindFile
	case 4:
		return store.KindURL
	case 5:
		return store.KindCode
	case 6:
		return store.KindRichText
	default:
		return ""
	}
}

func (a *clipApp) stopSearchTimer() {
	if a.searchTimer != nil {
		a.searchTimer.Stop()
		a.searchTimer = nil
	}
}

func (a *clipApp) resetScrollToTop() {
	a.controlsCollapsed = false
	a.atTop = true
	a.upScrollAccum = 0
	a.listScroll.Y = 0
}

func (a *clipApp) scrollListToTop() {
	if len(a.items) > 0 {
		a.list.ScrollTo(0, ui.Start)
	}
}

func (a *clipApp) syncSelectionBounds() {
	if len(a.items) == 0 {
		a.selected = -1
	} else if a.selected >= len(a.items) {
		a.selected = len(a.items) - 1
	}
}

func (a *clipApp) applyActiveTabFilter() {
	kind := a.currentKind()
	if kind == "" {
		a.items = a.searchHits
	} else {
		filtered := make([]store.ClipItem, 0, len(a.searchHits))
		for _, item := range a.searchHits {
			if item.Kind == kind {
				filtered = append(filtered, item)
			}
		}
		a.items = filtered
	}
	a.syncSelectionBounds()
}

func (a *clipApp) onTabChanged() {
	a.closeFileMenus()
	a.selected = -1
	a.resetScrollToTop()
	if strings.TrimSpace(a.query) != "" && a.searchTimer == nil && a.searchHits != nil {
		// 搜索态下切 Tab 直接在全局命中结果内按类别过滤，无需重复查库
		a.applyActiveTabFilter()
	} else {
		a.stopSearchTimer()
		a.reload()
	}
	a.scrollListToTop()
}

func (a *clipApp) runSearchReload() {
	a.searchTimer = nil
	a.selected = -1
	a.resetScrollToTop()
	a.reload()
	a.scrollListToTop()
}

func (a *clipApp) scheduleSearchReload() {
	a.stopSearchTimer()
	if a.panel == nil || strings.TrimSpace(a.query) == "" {
		a.runSearchReload()
		return
	}
	a.searchTimer = time.AfterFunc(160*time.Millisecond, func() {
		if a.panel != nil {
			a.panel.Update(a.runSearchReload)
		}
	})
}

func (a *clipApp) historyLimit() int {
	limit := 500
	if a.settings.MaxHistory > 0 {
		limit = max(a.settings.MaxHistory, 200)
	}
	return limit
}

func (a *clipApp) reloadIfStale() {
	if a.store == nil {
		return
	}
	rev := a.store.Revision()
	kind := a.currentKind()
	limit := a.historyLimit()
	if rev == a.loadedRevision &&
		a.loadedQuery == a.query &&
		a.loadedKind == kind &&
		a.loadedLimit == limit &&
		a.store.TagRevision() == a.loadedTagRevision {
		return
	}
	a.reload()
}

func (a *clipApp) reload() {
	if a.store == nil {
		return
	}
	limit := a.historyLimit()
	kind := a.currentKind()
	rev := a.store.Revision()
	if strings.TrimSpace(a.query) != "" {
		// 搜索时拉取全类别匹配结果，便于在不同 Tab 间即时切换查看
		hits, err := a.store.ListSummaries(a.query, "", limit)
		if err != nil {
			log.Printf("list clipboard items: %v", err)
			return
		}
		a.searchHits = hits
		a.applyActiveTabFilter()
	} else {
		a.searchHits = nil
		items, err := a.store.ListSummaries("", kind, limit)
		if err != nil {
			log.Printf("list clipboard items: %v", err)
			return
		}
		a.items = items
		a.syncSelectionBounds()
	}
	a.loadedRevision = rev
	a.loadedQuery = a.query
	a.loadedKind = kind
	a.loadedLimit = limit
	if tagRev := a.store.TagRevision(); tagRev != a.loadedTagRevision {
		if tags, err := a.store.ListAllTags(); err == nil {
			a.allTags = tags
			a.loadedTagRevision = tagRev
		}
	}
}

func (a *clipApp) moveSelection(delta int) {
	if len(a.items) == 0 {
		a.selected = -1
		return
	}
	if a.selected < 0 {
		a.selected = 0
		a.list.ScrollIntoView(0)
		return
	}
	next := a.selected + delta
	if next < 0 {
		next = 0
	}
	if next >= len(a.items) {
		next = len(a.items) - 1
	}
	a.selected = next
	a.list.ScrollIntoView(next)
}

func (a *clipApp) togglePin(id int64) {
	if a.store == nil {
		return
	}
	if _, err := a.store.TogglePin(id); err != nil {
		log.Printf("toggle pin: %v", err)
		return
	}
	a.reload()
}

func (a *clipApp) deleteItem(id int64) {
	if a.store == nil {
		return
	}
	a.closeFileMenus()
	if err := a.store.Delete(id); err != nil {
		log.Printf("delete item: %v", err)
		return
	}
	a.removeTempFilesForID(id)
	a.reload()
}

func (a *clipApp) clearUnpinned() {
	if a.store == nil {
		return
	}
	a.closeFileMenus()
	if err := a.store.ClearUnpinned(); err != nil {
		log.Printf("clear unpinned: %v", err)
		return
	}
	a.reload()
}

func (a *clipApp) clearAll() {
	if a.store == nil {
		return
	}
	a.closeFileMenus()
	if err := a.store.ClearAll(); err != nil {
		log.Printf("clear all: %v", err)
		return
	}
	a.cleanupTempFiles()
	a.thumbs.clear()
	a.richSpans = nil
	a.reload()
}

func (a *clipApp) persistSettings() {
	if a.persistTimer != nil {
		a.persistTimer.Stop()
		a.persistTimer = nil
	}
	a.persistPending = false
	if a.store == nil {
		return
	}
	if err := a.store.SaveSettings(a.settings); err != nil {
		log.Printf("save settings: %v", err)
	}
}

func (a *clipApp) schedulePersistSettings() {
	a.persistPending = true
	if a.persistTimer != nil {
		a.persistTimer.Reset(300 * time.Millisecond)
		return
	}
	a.persistTimer = time.AfterFunc(300*time.Millisecond, func() {
		if a.panel != nil {
			a.panel.Update(a.flushPendingSettingsPersist)
		}
	})
}

func (a *clipApp) flushPendingSettingsPersist() {
	if a.persistTimer != nil {
		a.persistTimer.Stop()
		a.persistTimer = nil
	}
	if !a.persistPending {
		return
	}
	a.persistSettings()
}

func (a *clipApp) resetSettings() {
	if a.store == nil {
		a.settings = store.DefaultSettings()
	} else {
		cfg, err := a.store.ResetSettings()
		if err != nil {
			log.Printf("reset settings: %v", err)
			return
		}
		a.settings = cfg
	}
	a.applyAutoHide(a.settings.AutoHideOnBlur)
	a.applyNoFocusMode(a.settings.NoFocusMode)
	a.applyBackdrop()
	if mygo.App.IsReady() {
		if err := mygo.App.SetOpenAtLogin(a.settings.AutoStart); err != nil {
			log.Printf("reset open at login: %v", err)
		}
		a.applyWinVOverride()
	}
	if a.panel != nil {
		a.panel.SetSize(a.settings.WindowWidth, a.settings.WindowHeight)
	}
	a.syncPlainPasteShortcut()
}

func (a *clipApp) hydrateItem(item store.ClipItem) store.ClipItem {
	if a.store == nil || item.ID == 0 {
		return item
	}
	needsText := item.Text == "" && (item.Kind == store.KindText || item.Kind == store.KindURL || item.Kind == store.KindCode || item.Kind == store.KindRichText)
	if !needsText && item.Kind != store.KindRichText {
		return item
	}
	if full, err := a.store.GetByID(item.ID); err == nil {
		return full
	}
	return item
}

var writeClipItemFn = writeClipItem

func (a *clipApp) pasteSelected() {
	a.pasteSelectedWith(false)
}

func (a *clipApp) pasteSelectedPlainText() {
	a.pasteSelectedWith(true)
}

func (a *clipApp) pasteSelectedWith(plainText bool) {
	if len(a.items) == 0 {
		return
	}
	idx := a.selected
	if idx < 0 || idx >= len(a.items) {
		idx = 0
	}
	a.pasteItem(a.items[idx], plainText)
}

func (a *clipApp) pasteItem(raw store.ClipItem, plainText bool) {
	item := a.hydrateItem(raw)
	if plainText && item.HTML != "" {
		a.ignoreHash = store.TextContentHash(item.Text, "")
	} else {
		a.ignoreHash = item.Hash
	}
	a.ignoreUntil = time.Now().Add(1500 * time.Millisecond)

	if mygo.App.IsReady() || writeClipItemFn != nil {
		if err := writeClipItemFn(item, plainText); err != nil {
			log.Printf("write clipboard: %v", err)
			return
		}
		if seq := sys.ClipboardSequenceNumber(); seq != 0 {
			a.ignoreClipSeq = seq
		}
		if item.Kind == store.KindImage && mygo.App.IsReady() {
			a.releaseImagePasteMemory()
		}
	}
	if !a.settings.ClickCopyOnly && a.settings.DeleteAfterPaste && a.store != nil {
		if err := a.store.Delete(item.ID); err != nil {
			log.Printf("delete after paste: %v", err)
		} else {
			a.reload()
		}
	} else if a.settings.MoveToTopOnPaste && a.store != nil {
		if err := a.store.TouchItem(item.ID); err != nil {
			log.Printf("touch item: %v", err)
		} else {
			a.reload()
		}
	}
	if a.settings.ClickCopyOnly {
		a.setStatus(locales.StatusCopied())
		return
	}
	wasNoActivate := a.panelNoActivate
	restoreFocus := a.winPinned && !wasNoActivate
	if a.panel != nil && !a.winPinned {
		a.hidePanel()
	} else if a.winPinned && a.settings.NoFocusMode && !wasNoActivate {
		a.setPanelNoActivate(true)
	}
	if mygo.App.IsReady() {
		method := a.settings.PasteMethod
		text := item.Text
		isTextKind := item.Kind == store.KindText || item.Kind == store.KindRichText || item.Kind == store.KindURL || item.Kind == store.KindCode
		go func() {
			if restoreFocus {
				sys.RestoreForegroundWindow()
				time.Sleep(70 * time.Millisecond)
			} else if wasNoActivate {
				time.Sleep(30 * time.Millisecond)
			} else {
				time.Sleep(60 * time.Millisecond)
			}
			sendFn := func() error {
				return sys.SendPasteWith(method, text, isTextKind)
			}
			var err error
			if method == "game_mode" && isTextKind && text != "" {
				err = sendFn()
			} else {
				err = runWithTimeout(pasteTimeout, sendFn)
			}
			if err != nil {
				log.Printf("send paste: %v", err)
			}
		}()
	}
}

func writeClipItem(item store.ClipItem, plainText bool) error {
	if !mygo.App.IsReady() {
		return nil
	}
	switch item.Kind {
	case store.KindText, store.KindRichText, store.KindURL, store.KindCode:
		if item.HTML != "" && !plainText {
			return mygo.Clipboard.Write(transfer.New(transfer.NewItem(
				transfer.Bytes(transfer.Text, []byte(item.Text)),
				transfer.Bytes(transfer.HTML, []byte(item.HTML)),
			)))
		}
		return mygo.Clipboard.Write(transfer.New(transfer.NewItem(
			transfer.Bytes(transfer.Text, []byte(item.Text)),
		)))
	case store.KindImage:
		if err := sys.WriteClipboardImageFile(item.ImagePath); err == nil {
			return nil
		} else if !errors.Is(err, sys.ErrClipboardUnsupported) {
			return err
		}
		b, err := os.ReadFile(item.ImagePath)
		if err != nil {
			return err
		}
		return mygo.Clipboard.Write(transfer.New(transfer.NewItem(
			transfer.Bytes(transfer.PNG, b),
		)))
	case store.KindFile:
		return mygo.Clipboard.WriteFiles(item.Files...)
	default:
		return nil
	}
}

func (a *clipApp) onClipboardChanged() {
	if seq := sys.ClipboardSequenceNumber(); seq != 0 && seq == a.ignoreClipSeq && time.Now().Before(a.ignoreUntil) {
		return
	}
	formats := mygo.Clipboard.Formats()
	if len(formats) == 0 {
		return
	}
	pref, ok := transfer.PreferredFormat(formats, transfer.FileList, transfer.URIList, transfer.PNG, transfer.HTML, transfer.Text)
	if !ok {
		return
	}

	sourceApp, sourcePath := sys.CaptureClipboardSource()
	opts := store.SaveOptions{
		Deduplicate: a.settings.Deduplicate,
		MaxHistory:  a.settings.MaxHistory,
		SourceApp:   sourceApp,
		SourcePath:  sourcePath,
	}

	switch pref {
	case transfer.FileList, transfer.URIList:
		if !a.settings.CaptureFiles {
			return
		}
		files, err := mygo.Clipboard.ReadFiles()
		if err != nil || len(files) == 0 {
			return
		}
		if h, err := store.FilesContentHash(files); err == nil && a.shouldIgnore(h) {
			return
		}
		a.saveAsync(func() (string, error) {
			it, err := a.store.SaveFiles(files, opts)
			return it.Hash, err
		})
	case transfer.PNG:
		data, err := mygo.Clipboard.ReadFormat(transfer.PNG)
		if err != nil || len(data) == 0 {
			return
		}
		sum := sha256.Sum256(data)
		h := hex.EncodeToString(sum[:])
		if a.shouldIgnore(h) {
			return
		}
		a.saveAsync(func() (string, error) {
			it, err := a.store.SaveImage(data, opts)
			return it.Hash, err
		})
	case transfer.HTML, transfer.Text:
		data, err := mygo.Clipboard.ReadFormat(transfer.Text)
		if err != nil || len(data) == 0 {
			return
		}
		text := string(data)
		if strings.TrimSpace(text) == "" {
			return
		}
		var htmlStr string
		if a.settings.CaptureRichText && slices.Contains(formats, transfer.HTML) {
			if htmlData, err := mygo.Clipboard.ReadFormat(transfer.HTML); err == nil && len(htmlData) > 0 {
				htmlStr = normalizeCapturedHTML(htmlData, text)
			}
		}
		h := store.TextContentHash(text, htmlStr)
		if a.shouldIgnore(h) {
			return
		}
		opts.HTML = htmlStr
		snap := privacySnapshot{
			enabled:     a.settings.PrivacyProtection,
			kinds:       slices.Clone(a.settings.PrivacyKinds),
			customRules: a.settings.PrivacyCustomRules,
		}
		a.saveAsync(func() (string, error) {
			it, err := a.saveTextWithPrivacySnapshot(text, opts, snap)
			return it.Hash, err
		})
	}
}

type privacySnapshot struct {
	enabled     bool
	kinds       []string
	customRules string
}

func normalizeCapturedHTML(htmlData []byte, plainText string) string {
	htmlStr := decodeClipboardHTML(htmlData)
	if htmlStr != "" && !isMeaningfulRichHTML(htmlStr, plainText) {
		return ""
	}
	return htmlStr
}

func (a *clipApp) saveTextWithPrivacy(text string, opts store.SaveOptions) (store.ClipItem, error) {
	return a.saveTextWithPrivacySnapshot(text, opts, privacySnapshot{
		enabled:     a.settings.PrivacyProtection,
		kinds:       a.settings.PrivacyKinds,
		customRules: a.settings.PrivacyCustomRules,
	})
}

func (a *clipApp) saveTextWithPrivacySnapshot(text string, opts store.SaveOptions, snap privacySnapshot) (store.ClipItem, error) {
	if snap.enabled && store.ContainsSensitiveInfo(text, snap.kinds, snap.customRules) {
		opts.Tags = append(opts.Tags, store.SensitiveTag)
	}
	return a.store.SaveText(text, opts)
}

func (a *clipApp) shouldIgnore(h string) bool {
	return a.ignoreHash != "" && h == a.ignoreHash && time.Now().Before(a.ignoreUntil)
}

func (a *clipApp) saveAsync(fn func() (string, error)) {
	if fn == nil {
		return
	}
	a.saveMu.Lock()
	a.saveQueue = append(a.saveQueue, fn)
	if a.saveRunning {
		a.saveMu.Unlock()
		return
	}
	a.saveRunning = true
	a.saveMu.Unlock()
	go a.drainSaveQueue()
}

func (a *clipApp) drainSaveQueue() {
	for {
		a.saveMu.Lock()
		if len(a.saveQueue) == 0 {
			a.saveQueue = nil
			a.saveRunning = false
			a.saveMu.Unlock()
			return
		}
		fn := a.saveQueue[0]
		a.saveQueue[0] = nil
		a.saveQueue = a.saveQueue[1:]
		a.saveMu.Unlock()

		err := runWithTimeout(saveAsyncTimeout, func() error {
			_, err := fn()
			return err
		})
		if err != nil {
			log.Printf("save clipboard item: %v", err)
			continue
		}
		if a.panel != nil {
			panel := a.panel
			panel.Update(func() {
				if !panel.IsVisible() {
					if a.loadedRevision != 0 {
						a.settleHiddenState()
					}
					return
				}
				wasAtTop := a.listScroll.Y <= atTopEpsilon
				a.reload()
				if wasAtTop && len(a.items) > 0 {
					a.listScroll.Y = 0
					a.list.ScrollTo(0, ui.Start)
				}
			})
		}
	}
}
