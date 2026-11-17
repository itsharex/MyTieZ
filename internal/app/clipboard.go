package app

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"os"
	"strings"
	"time"

	"mytiez/internal/store"
	"mytiez/internal/sys"

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
	default:
		return ""
	}
}

func (a *clipApp) reload() {
	if a.store == nil {
		return
	}
	limit := 500
	if a.settings.MaxHistory > 0 {
		limit = max(a.settings.MaxHistory, 200)
	}
	items, err := a.store.List(a.query, a.currentKind(), limit)
	if err != nil {
		log.Printf("list clipboard items: %v", err)
		return
	}
	a.items = items
	if len(a.items) == 0 {
		a.selected = -1
	} else if a.selected >= len(a.items) {
		a.selected = len(a.items) - 1
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
	if err := a.store.Delete(id); err != nil {
		log.Printf("delete item: %v", err)
		return
	}
	a.reload()
}

func (a *clipApp) clearUnpinned() {
	if a.store == nil {
		return
	}
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
	if err := a.store.ClearAll(); err != nil {
		log.Printf("clear all: %v", err)
		return
	}
	a.thumbs.clear()
	a.reload()
}

func (a *clipApp) persistSettings() {
	if a.store == nil {
		return
	}
	if err := a.store.SaveSettings(a.settings); err != nil {
		log.Printf("save settings: %v", err)
	}
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
	a.applyBackdrop()
	if mygo.App.IsReady() {
		if err := mygo.App.SetOpenAtLogin(a.settings.AutoStart); err != nil {
			log.Printf("reset open at login: %v", err)
		}
		_ = a.switchHotkey(a.settings.Hotkey)
	}
	if a.panel != nil {
		a.panel.SetSize(a.settings.WindowWidth, a.settings.WindowHeight)
	}
	a.settingsNotice = "已恢复默认设置"
}

func (a *clipApp) pasteSelected() {
	if len(a.items) == 0 {
		return
	}
	idx := a.selected
	if idx < 0 || idx >= len(a.items) {
		idx = 0
	}
	item := a.items[idx]
	a.ignoreHash = item.Hash
	a.ignoreUntil = time.Now().Add(1500 * time.Millisecond)

	if err := writeClipItem(item); err != nil {
		log.Printf("write clipboard: %v", err)
		return
	}
	if a.settings.MoveToTopOnPaste && a.store != nil {
		if err := a.store.TouchItem(item.ID); err != nil {
			log.Printf("touch item: %v", err)
		} else {
			a.reload()
		}
	}
	if a.settings.ClickCopyOnly {
		a.statusMsg = "已复制到剪贴板"
		a.statusUntil = time.Now().Add(2 * time.Second)
		return
	}
	if a.panel != nil {
		a.panel.Hide()
	}
	go func() {
		time.Sleep(60 * time.Millisecond)
		if err := runWithTimeout(pasteTimeout, sys.SendPaste); err != nil {
			log.Printf("send paste: %v", err)
		}
	}()
}

func writeClipItem(item store.ClipItem) error {
	switch item.Kind {
	case store.KindText:
		return mygo.Clipboard.Write(transfer.New(transfer.NewItem(
			transfer.Bytes(transfer.Text, []byte(item.Text)),
		)))
	case store.KindImage:
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
	formats := mygo.Clipboard.Formats()
	if len(formats) == 0 {
		return
	}
	pref, ok := transfer.PreferredFormat(formats, transfer.FileList, transfer.URIList, transfer.PNG, transfer.Text)
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
	case transfer.Text:
		data, err := mygo.Clipboard.ReadFormat(transfer.Text)
		if err != nil || len(data) == 0 {
			return
		}
		text := string(data)
		if strings.TrimSpace(text) == "" {
			return
		}
		sum := sha256.Sum256([]byte(text))
		h := hex.EncodeToString(sum[:])
		if a.shouldIgnore(h) {
			return
		}
		a.saveAsync(func() (string, error) {
			it, err := a.store.SaveText(text, opts)
			return it.Hash, err
		})
	}
}

func (a *clipApp) shouldIgnore(h string) bool {
	return a.ignoreHash != "" && h == a.ignoreHash && time.Now().Before(a.ignoreUntil)
}

func (a *clipApp) saveAsync(fn func() (string, error)) {
	go func() {
		err := runWithTimeout(saveAsyncTimeout, func() error {
			_, err := fn()
			return err
		})
		if err != nil {
			log.Printf("save clipboard item: %v", err)
			return
		}
		if a.panel != nil {
			a.panel.Update(func() {
				wasAtTop := a.listScroll.Y <= atTopEpsilon
				a.reload()
				if wasAtTop && len(a.items) > 0 {
					a.listScroll.Y = 0
					a.list.ScrollTo(0, ui.Start)
				}
			})
		}
	}()
}
