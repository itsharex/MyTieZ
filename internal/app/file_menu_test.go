package app

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"mytiez/internal/store"
	"mytiez/internal/sys"
	"mytiez/locales"

	"github.com/egoist/mygo/ui"
)

func stubShellOpenAndFolder(t *testing.T) (opened *[]string, shown *[]string) {
	t.Helper()
	var openedPaths []string
	var shownPaths []string

	origOpen := shellOpenPath
	origShow := shellShowItemInFolder
	shellOpenPath = func(p string) error {
		openedPaths = append(openedPaths, p)
		return nil
	}
	shellShowItemInFolder = func(p string) {
		shownPaths = append(shownPaths, p)
	}
	t.Cleanup(func() {
		shellOpenPath = origOpen
		shellShowItemInFolder = origShow
	})
	return &openedPaths, &shownPaths
}

func newTestAppWithFiles(t *testing.T, groups ...[]string) *clipApp {
	t.Helper()
	st, err := store.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = st.Close()
	})
	for _, files := range groups {
		if _, err := st.UpsertFiles(files); err != nil {
			t.Fatal(err)
		}
	}
	a := newClipApp(st, nil)
	a.reload()
	return a
}

func clickCenter(tt *ui.Tester, b ui.Rect) {
	tt.ClickAt(b.X+b.W/2, b.Y+b.H/2)
}

func findMenuItemsByName(tt *ui.Tester, name string) []ui.AccessInfo {
	var out []ui.AccessInfo
	for _, info := range tt.AccessAll() {
		if info.Role == ui.RoleMenuItem && info.Name == name {
			out = append(out, info)
		}
	}
	return out
}

func TestOpenSingleFileDirectly(t *testing.T) {
	opened, shown := stubShellOpenAndFolder(t)
	dir := t.TempDir()
	existing := filepath.Join(dir, "report.pdf")
	if err := os.WriteFile(existing, []byte("pdf"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "gone.pdf")

	a := newTestAppWithFiles(t, []string{existing})
	a.routedSearchActive = true
	tt := ui.NewTester(a.view, panelWidth, panelHeight)

	if err := tt.Click(locales.In("zh-Hans").CardActionOpen()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(*opened, []string{existing}) {
		t.Fatalf("expected single file opened %q, got %q", []string{existing}, *opened)
	}
	if len(*shown) != 0 {
		t.Fatalf("expected no folder reveal on single open click, got %q", *shown)
	}
	if a.routedSearchActive {
		t.Fatalf("expected hidePanel called after opening existing file")
	}

	a2 := newTestAppWithFiles(t, []string{missing})
	a2.routedSearchActive = true
	a2.openItem(a2.items[0])
	if len(*opened) != 1 {
		t.Fatalf("expected missing file not opened, opened=%q", *opened)
	}
	if a2.pendingToast != locales.StatusFileMissing() {
		t.Fatalf("expected missing file toast %q, got %q", locales.StatusFileMissing(), a2.pendingToast)
	}
	if !a2.routedSearchActive {
		t.Fatalf("expected hidePanel not called when file is missing")
	}
}

func TestOpenItemMultiFileFallbackDoesNotOpenAll(t *testing.T) {
	opened, shown := stubShellOpenAndFolder(t)
	dir := t.TempDir()
	missing := filepath.Join(dir, "00-missing.txt")
	first := filepath.Join(dir, "01-first.txt")
	second := filepath.Join(dir, "02-second.txt")
	for _, p := range []string{first, second} {
		if err := os.WriteFile(p, []byte("ok"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	a := newTestAppWithFiles(t, []string{missing, first, second})
	a.routedSearchActive = true
	a.openItem(a.items[0])

	if len(*opened) != 0 {
		t.Fatalf("expected multi-file openItem fallback not to call OpenPath, got %q", *opened)
	}
	if !slices.Equal(*shown, []string{first}) {
		t.Fatalf("expected multi-file openItem fallback to reveal %q, got %q", first, *shown)
	}
	if a.routedSearchActive {
		t.Fatalf("expected hidePanel called after revealing multi-file item in folder")
	}
}

func TestMultiFileOpenMenuInteractions(t *testing.T) {
	opened, shown := stubShellOpenAndFolder(t)
	msgs := locales.In("zh-Hans")

	t.Run("same directory with missing item", func(t *testing.T) {
		*opened = nil
		*shown = nil

		dir := t.TempDir()
		f1 := filepath.Join(dir, "alpha.txt")
		f2 := filepath.Join(dir, "beta.txt")
		fMissing := filepath.Join(dir, "deleted.txt")
		for _, p := range []string{f1, f2} {
			if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}

		a := newTestAppWithFiles(t, []string{f1, f2, fMissing})
		tt := ui.NewTester(a.view, panelWidth, panelHeight)

		if err := tt.Click(msgs.CardActionOpenFiles()); err != nil {
			t.Fatal(err)
		}
		if len(*opened) != 0 || len(*shown) != 0 {
			t.Fatalf("opening menu must not open files immediately, opened=%q shown=%q", *opened, *shown)
		}
		if !a.hasOpenFileMenu() {
			t.Fatalf("expected file menu open after clicking open button")
		}

		showInfo, ok := tt.AccessByRole(ui.RoleMenuItem, msgs.FileMenuShowInFolder())
		if !ok || showInfo.Disabled {
			t.Fatalf("expected enabled Show in Folder menu item, got ok=%v info=%+v", ok, showInfo)
		}
		if showInfo.Bounds.W > fileMenuMultiMaxWidth || showInfo.Bounds.H > 32 {
			t.Fatalf("expected compact single-line Show in Folder item (W<=%.0f, H<=32), got bounds=%+v", fileMenuMultiMaxWidth, showInfo.Bounds)
		}
		missingInfo, ok := tt.AccessByRole(ui.RoleMenuItem, "deleted.txt")
		if !ok || !missingInfo.Disabled {
			t.Fatalf("expected deleted.txt menu item disabled, got ok=%v info=%+v", ok, missingInfo)
		}
		betaInfo, ok := tt.AccessByRole(ui.RoleMenuItem, "beta.txt")
		if !ok || betaInfo.Disabled {
			t.Fatalf("expected beta.txt menu item enabled, got ok=%v info=%+v", ok, betaInfo)
		}

		clickCenter(tt, betaInfo.Bounds)
		if !slices.Equal(*opened, []string{f2}) {
			t.Fatalf("expected only beta.txt (%q) opened, got %q", f2, *opened)
		}
		if a.hasOpenFileMenu() {
			t.Fatalf("expected menu closed after selecting file")
		}

		// Re-open menu and trigger Show in Folder.
		if err := tt.Click(msgs.CardActionOpenFiles()); err != nil {
			t.Fatal(err)
		}
		showInfo, ok = tt.AccessByRole(ui.RoleMenuItem, msgs.FileMenuShowInFolder())
		if !ok {
			t.Fatalf("expected Show in Folder menu item")
		}
		clickCenter(tt, showInfo.Bounds)
		if !slices.Equal(*shown, []string{f1}) {
			t.Fatalf("expected ShowItemInFolder(%q), got %q", f1, *shown)
		}
	})

	t.Run("cross directory with duplicate names", func(t *testing.T) {
		*opened = nil
		*shown = nil

		root := t.TempDir()
		dirA := filepath.Join(root, "dirA")
		dirB := filepath.Join(root, "dirB")
		for _, d := range []string{dirA, dirB} {
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		readmeA := filepath.Join(dirA, "readme.md")
		readmeB := filepath.Join(dirB, "readme.md")
		for _, p := range []string{readmeA, readmeB} {
			if err := os.WriteFile(p, []byte("doc"), 0o600); err != nil {
				t.Fatal(err)
			}
		}

		a := newTestAppWithFiles(t, []string{readmeA, readmeB})
		tt := ui.NewTester(a.view, panelWidth, panelHeight)

		if err := tt.Click(msgs.CardActionOpenFiles()); err != nil {
			t.Fatal(err)
		}
		dupItems := findMenuItemsByName(tt, "readme.md")
		if len(dupItems) != 2 {
			t.Fatalf("expected 2 readme.md menu items, got %d (%+v)", len(dupItems), dupItems)
		}
		var targetB ui.AccessInfo
		var foundB bool
		for _, it := range dupItems {
			if strings.Contains(it.Description, dirB) {
				targetB = it
				foundB = true
				break
			}
		}
		if !foundB {
			t.Fatalf("expected second readme.md item to describe %q, got %+v", dirB, dupItems)
		}
		clickCenter(tt, targetB.Bounds)
		if !slices.Equal(*opened, []string{readmeB}) {
			t.Fatalf("expected second readme.md (%q) opened, got %q", readmeB, *opened)
		}

		// Verify flat Show in Folder entries fit inside the 400px panel and reveal dirB.
		if err := tt.Click(msgs.CardActionOpenFiles()); err != nil {
			t.Fatal(err)
		}
		dirBItem, ok := tt.AccessByRole(ui.RoleMenuItem, msgs.FileMenuShowNamedFolder("dirB"))
		if !ok {
			t.Fatalf("expected %q menu item, menu=%q", msgs.FileMenuShowNamedFolder("dirB"), tt.Menu())
		}
		if dirBItem.Bounds.X < 0 || dirBItem.Bounds.X+dirBItem.Bounds.W > panelWidth {
			t.Fatalf("expected dirB menu item within panel width %d, got bounds=%+v", panelWidth, dirBItem.Bounds)
		}
		clickCenter(tt, dirBItem.Bounds)
		if !slices.Equal(*shown, []string{readmeB}) {
			t.Fatalf("expected dirB reveal %q, got %q", readmeB, *shown)
		}
	})
}

func TestSingleFileContextMenuShowInFolder(t *testing.T) {
	opened, shown := stubShellOpenAndFolder(t)
	msgs := locales.In("zh-Hans")

	dir := t.TempDir()
	single := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(single, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	a := newTestAppWithFiles(t, []string{single})
	a.settings.ClickCopyOnly = true
	tt := ui.NewTester(a.view, panelWidth, panelHeight)

	rect, ok := tt.Find("notes.txt")
	if !ok {
		t.Fatalf("expected notes.txt in view, texts=%q", tt.Texts())
	}
	tt.RightClickAt(rect.X+10, rect.Y+rect.H/2)

	showInfo, ok := tt.AccessByRole(ui.RoleMenuItem, msgs.FileMenuShowInFolder())
	if !ok {
		t.Fatalf("expected %q in context menu, menu=%q", msgs.FileMenuShowInFolder(), tt.Menu())
	}
	clickCenter(tt, showInfo.Bounds)
	if !slices.Equal(*shown, []string{single}) {
		t.Fatalf("expected ShowItemInFolder(%q), got %q", single, *shown)
	}
	if len(*opened) != 0 {
		t.Fatalf("expected no OpenPath call, got %q", *opened)
	}
	if a.pendingToast != "" {
		t.Fatalf("expected context menu action not to trigger card paste toast, got %q", a.pendingToast)
	}
}

func TestFileMenuHideCardActionsAndNoFocusEscape(t *testing.T) {
	opened, _ := stubShellOpenAndFolder(t)
	msgs := locales.In("zh-Hans")

	dir := t.TempDir()
	f1 := filepath.Join(dir, "one.txt")
	f2 := filepath.Join(dir, "two.txt")
	for _, p := range []string{f1, f2} {
		if err := os.WriteFile(p, []byte("v"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	a := newTestAppWithFiles(t, []string{f1, f2})
	a.settings.HideCardActions = true
	tt := ui.NewTester(a.view, panelWidth, panelHeight)

	cardRect, ok := tt.Find("one.txt")
	if !ok {
		t.Fatalf("expected one.txt in view")
	}
	// Hover card so action buttons appear, then click the multi-file menu button.
	tt.Move(cardRect.X+20, cardRect.Y+cardRect.H/2)
	btnInfo, ok := tt.AccessByRole(ui.RoleMenuButton, msgs.CardActionOpenFiles())
	if !ok {
		t.Fatalf("expected file menu button visible while hovered")
	}
	clickCenter(tt, btnInfo.Bounds)
	if !a.hasOpenFileMenu() {
		t.Fatalf("expected file menu open after clicking button")
	}

	// Move pointer away from the card so card.Hovered() becomes false; menu must stay open.
	tt.Move(10, panelHeight-10)
	twoInfo, ok := tt.AccessByRole(ui.RoleMenuItem, "two.txt")
	if !ok {
		t.Fatalf("expected menu to remain open when pointer leaves card under HideCardActions")
	}

	// Simulate Escape under no-focus key routing: closes menu without pasting.
	a.applyRoutedKey(sys.RoutedKey{VK: vkEscape})
	tt.Frame()
	if a.hasOpenFileMenu() {
		t.Fatalf("expected Escape in routed keys to close file menu")
	}
	if len(*opened) != 0 {
		t.Fatalf("expected Escape not to open files, got %q", *opened)
	}

	// Re-open via hover, leave card, and click two.txt to ensure it remains clickable.
	tt.Move(cardRect.X+20, cardRect.Y+cardRect.H/2)
	btnInfo, ok = tt.AccessByRole(ui.RoleMenuButton, msgs.CardActionOpenFiles())
	if !ok {
		t.Fatalf("expected file menu button visible on re-hover")
	}
	clickCenter(tt, btnInfo.Bounds)
	tt.Move(10, panelHeight-10)
	twoInfo, ok = tt.AccessByRole(ui.RoleMenuItem, "two.txt")
	if !ok {
		t.Fatalf("expected two.txt menu item after moving pointer away")
	}
	clickCenter(tt, twoInfo.Bounds)
	if !slices.Equal(*opened, []string{f2}) {
		t.Fatalf("expected only two.txt (%q) opened, got %q", f2, *opened)
	}
}

func TestMultiFileMenuOmitsExcessFiles(t *testing.T) {
	stubShellOpenAndFolder(t)
	msgs := locales.In("zh-Hans")

	dir := t.TempDir()
	files := make([]string, 0, 55)
	for i := 1; i <= 55; i++ {
		files = append(files, filepath.Join(dir, fmt.Sprintf("item-%02d.txt", i)))
	}

	a := newTestAppWithFiles(t, files)
	tt := ui.NewTester(a.view, panelWidth, panelHeight)

	if err := tt.Click(msgs.CardActionOpenFiles()); err != nil {
		t.Fatal(err)
	}

	if _, ok := tt.AccessByRole(ui.RoleMenuItem, "item-50.txt"); !ok {
		t.Fatalf("expected item-50.txt in menu")
	}
	if _, ok := tt.AccessByRole(ui.RoleMenuItem, "item-51.txt"); ok {
		t.Fatalf("expected item-51.txt omitted past maxFileMenuEntries")
	}
	omittedMsg := msgs.FileMenuMoreOmitted(5)
	moreInfo, ok := tt.AccessByRole(ui.RoleMenuItem, omittedMsg)
	if !ok || !moreInfo.Disabled {
		t.Fatalf("expected disabled omitted notice %q, got ok=%v info=%+v", omittedMsg, ok, moreInfo)
	}
}
