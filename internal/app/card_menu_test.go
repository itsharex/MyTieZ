package app

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"mytiez/internal/store"
	"mytiez/internal/sys"
	"mytiez/locales"

	"github.com/egoist/mygo/ui"
)

type recordedWrite struct {
	item      store.ClipItem
	plainText bool
}

func stubWriteClipItem(t *testing.T) *[]recordedWrite {
	t.Helper()
	var writes []recordedWrite
	orig := writeClipItemFn
	writeClipItemFn = func(item store.ClipItem, plainText bool) error {
		writes = append(writes, recordedWrite{item: item, plainText: plainText})
		return nil
	}
	t.Cleanup(func() { writeClipItemFn = orig })
	return &writes
}

func stubShellOpenAndFolder(t *testing.T) (opened *[]string, shown *[]string) {
	t.Helper()
	var openCalls, showCalls []string
	origOpen, origShow := shellOpenPath, shellShowItemInFolder
	shellOpenPath = func(p string) error { openCalls = append(openCalls, p); return nil }
	shellShowItemInFolder = func(p string) { showCalls = append(showCalls, p) }
	t.Cleanup(func() {
		shellOpenPath = origOpen
		shellShowItemInFolder = origShow
	})
	return &openCalls, &showCalls
}

func newTestAppWithStore(t *testing.T) (*store.Store, *clipApp) {
	t.Helper()
	st, err := store.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st, newClipApp(st, nil)
}

func newTestAppWithFiles(t *testing.T, groups ...[]string) *clipApp {
	t.Helper()
	st, a := newTestAppWithStore(t)
	for _, g := range groups {
		if _, err := st.UpsertFiles(g); err != nil {
			t.Fatal(err)
		}
	}
	a.reload()
	return a
}

func newTestAppWithRichText(t *testing.T, text, html string) *clipApp {
	t.Helper()
	st, a := newTestAppWithStore(t)
	if _, err := st.SaveText(text, store.SaveOptions{HTML: html}); err != nil {
		t.Fatal(err)
	}
	a.reload()
	return a
}

func saveTestImage(t *testing.T, st *store.Store) store.ClipItem {
	t.Helper()
	tmp := filepath.Join(t.TempDir(), "sample.png")
	f, err := os.Create(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 12, 12))); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	_ = f.Close()
	data, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatal(err)
	}
	it, err := st.SaveImage(data, store.SaveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return it
}

func clickCenter(tt *ui.Tester, b ui.Rect) {
	tt.ClickAt(b.X+b.W/2, b.Y+b.H/2)
}

func TestUnifiedContextMenuAcrossAllCardKinds(t *testing.T) {
	msgs := locales.In("zh-Hans")
	opened, shown := stubShellOpenAndFolder(t)
	dir := t.TempDir()
	singleFile := filepath.Join(dir, "single-doc.txt")
	multiA := filepath.Join(dir, "multi-a.txt")
	multiB := filepath.Join(dir, "multi-b.txt")
	for _, p := range []string{singleFile, multiA, multiB} {
		if err := os.WriteFile(p, []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	st, a := newTestAppWithStore(t)
	_, _ = st.SaveText("plain text card", store.SaveOptions{})
	_, _ = st.SaveText("https://example.com/docs", store.SaveOptions{})
	_, _ = st.SaveText("func main() {\n\tfmt.Println(1)\n}", store.SaveOptions{})
	_, _ = st.SaveText("rich card", store.SaveOptions{HTML: "<p><b>rich</b> card</p>"})
	imgItem := saveTestImage(t, st)
	_, _ = st.UpsertFiles([]string{singleFile})
	_, _ = st.UpsertFiles([]string{multiA, multiB})
	a.reload()

	tt := ui.NewTester(a.view, panelWidth, 1200)
	baseMenu := []string{msgs.CardActionOpen(), msgs.CardMenuEditTags(), msgs.CardActionPin(), "-", msgs.CardActionDelete()}

	for _, tc := range []struct {
		find string
		want []string
	}{
		{"plain text card", baseMenu},
		{"https://example.com/docs", baseMenu},
		{"func main() { \tfmt.Println(1) }", baseMenu},
		{"rich card", []string{msgs.CardActionOpen(), msgs.RichMenuPastePlain(), msgs.CardMenuEditTags(), msgs.CardActionPin(), "-", msgs.CardActionDelete()}},
		{"single-doc.txt", []string{msgs.CardActionOpen(), msgs.FileMenuShowInFolder(), msgs.CardMenuEditTags(), msgs.CardActionPin(), "-", msgs.CardActionDelete()}},
		{"multi-a.txt", baseMenu},
	} {
		rect, ok := tt.Find(tc.find)
		if !ok {
			t.Fatalf("expected %q in view", tc.find)
		}
		tt.RightClickAt(rect.X+16, rect.Y+rect.H/2)
		if got := tt.Menu(); !slices.Equal(got, tc.want) {
			t.Fatalf("%q: expected menu %q, got %q", tc.find, tc.want, got)
		}
		if tc.find == "multi-a.txt" {
			openSub, _ := tt.AccessByRole(ui.RoleMenuItem, msgs.CardActionOpen())
			tt.Move(openSub.Bounds.X+openSub.Bounds.W/2, openSub.Bounds.Y+openSub.Bounds.H/2)
			if _, ok := tt.AccessByRole(ui.RoleMenuItem, "multi-a.txt"); !ok {
				t.Fatalf("expected multi-a.txt inside multi-file Open submenu")
			}
			tt.Key(0, ui.KeyEscape)
		}
		if tc.find == "single-doc.txt" {
			showInfo, _ := tt.AccessByRole(ui.RoleMenuItem, msgs.FileMenuShowInFolder())
			clickCenter(tt, showInfo.Bounds)
			if !slices.Equal(*shown, []string{singleFile}) || len(*opened) != 0 || a.pendingToast != "" {
				t.Fatalf("expected ShowInFolder(%q) without open/toast, got shown=%q opened=%q toast=%q", singleFile, *shown, *opened, a.pendingToast)
			}
		} else {
			tt.Key(0, ui.KeyEscape)
		}
	}

	// Image card (filtered to image tab so we target it reliably).
	if err := tt.Click(msgs.TabImage()); err != nil {
		t.Fatal(err)
	}
	imgLabel := imgItem.SourceApp
	if imgLabel == "" {
		imgLabel = "Unknown"
	}
	rect, ok := tt.Find(imgLabel)
	if !ok {
		t.Fatalf("expected image card %q in image tab", imgLabel)
	}
	tt.RightClickAt(rect.X+16, rect.Y+28)
	if got := tt.Menu(); !slices.Equal(got, baseMenu) {
		t.Fatalf("image: expected menu %q, got %q", baseMenu, got)
	}
	tt.Key(0, ui.KeyEscape)
}

func TestUnifiedContextMenuActions(t *testing.T) {
	opened, _ := stubShellOpenAndFolder(t)
	msgs := locales.In("zh-Hans")

	st, a := newTestAppWithStore(t)
	item, err := st.SaveText("action target text", store.SaveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	a.reload()
	tt := ui.NewTester(a.view, panelWidth, panelHeight)

	rect, ok := tt.Find("action target text")
	if !ok {
		t.Fatalf("expected card in view")
	}

	// Open
	tt.RightClickAt(rect.X+16, rect.Y+rect.H/2)
	openInfo, _ := tt.AccessByRole(ui.RoleMenuItem, msgs.CardActionOpen())
	clickCenter(tt, openInfo.Bounds)
	wantName := fmt.Sprintf("MyTieZ_entry_%d.txt", item.ID)
	if len(*opened) != 1 || filepath.Base((*opened)[0]) != wantName {
		t.Fatalf("expected open %q, got %q", wantName, *opened)
	}

	// Edit Tags opens card editor and closes menu
	tt.RightClickAt(rect.X+16, rect.Y+rect.H/2)
	editInfo, _ := tt.AccessByRole(ui.RoleMenuItem, msgs.CardMenuEditTags())
	clickCenter(tt, editInfo.Bounds)
	if a.editingTagsID != item.ID || len(tt.Menu()) != 0 {
		t.Fatalf("expected tag editor open (%d) and menu closed, got id=%d menu=%q", item.ID, a.editingTagsID, tt.Menu())
	}
	if err := tt.Click(msgs.TagsDoneA11y()); err != nil {
		t.Fatal(err)
	}

	// Pin -> Unpin
	tt.RightClickAt(rect.X+16, rect.Y+rect.H/2)
	pinInfo, _ := tt.AccessByRole(ui.RoleMenuItem, msgs.CardActionPin())
	clickCenter(tt, pinInfo.Bounds)
	if !a.items[0].Pinned {
		t.Fatalf("expected item pinned")
	}
	tt.RightClickAt(rect.X+16, rect.Y+rect.H/2)
	unpinInfo, ok := tt.AccessByRole(ui.RoleMenuItem, msgs.CardMenuUnpin())
	if !ok {
		t.Fatalf("expected Unpin menu item, menu=%q", tt.Menu())
	}
	clickCenter(tt, unpinInfo.Bounds)
	if a.items[0].Pinned {
		t.Fatalf("expected item unpinned")
	}

	// Delete
	tt.RightClickAt(rect.X+16, rect.Y+rect.H/2)
	delInfo, _ := tt.AccessByRole(ui.RoleMenuItem, msgs.CardActionDelete())
	clickCenter(tt, delInfo.Bounds)
	if len(a.items) != 0 {
		t.Fatalf("expected item deleted, remaining=%d", len(a.items))
	}
}

func TestQuickTagSubmenuAndBoundaryFlip(t *testing.T) {
	msgs := locales.In("zh-Hans")
	st, a := newTestAppWithStore(t)
	longTag := "very-long-tag-name-for-boundary-check"
	tagged, _ := st.SaveText("seed tagged item", store.SaveOptions{})
	_, _ = st.UpdateTags(tagged.ID, []string{"work", longTag})
	target, _ := st.SaveText("target card for quick tag", store.SaveOptions{})
	_, _ = st.UpdateTags(target.ID, []string{"work"})
	a.reload()

	tt := ui.NewTester(a.view, panelWidth, panelHeight)
	rect, ok := tt.Find("target card for quick tag")
	if !ok {
		t.Fatalf("expected target card in view")
	}

	// Right-edge click: submenu must flip left and stay within [0, 400].
	tt.RightClickAt(panelWidth-40, rect.Y+rect.H/2)
	quickInfo, ok := tt.AccessByRole(ui.RoleMenuItem, msgs.CardMenuQuickTag())
	if !ok {
		t.Fatalf("expected Quick Tag submenu trigger, menu=%q", tt.Menu())
	}
	tt.Move(quickInfo.Bounds.X+quickInfo.Bounds.W/2, quickInfo.Bounds.Y+quickInfo.Bounds.H/2)

	workItem, ok := tt.AccessByRole(ui.RoleMenuItem, "work")
	if !ok || workItem.Disabled {
		t.Fatalf("expected applied tag 'work' enabled, got ok=%v info=%+v", ok, workItem)
	}
	longItem, ok := tt.AccessByRole(ui.RoleMenuItem, longTag)
	if !ok || longItem.Disabled || longItem.Bounds.X >= quickInfo.Bounds.X {
		t.Fatalf("expected unapplied tag enabled and flipped left, got ok=%v sub=%+v trigger=%+v", ok, longItem, quickInfo)
	}
	for _, info := range []ui.AccessInfo{workItem, longItem} {
		if info.Bounds.X < 0 || info.Bounds.X+info.Bounds.W > panelWidth || info.Bounds.W > cardMenuMaxWidth {
			t.Fatalf("submenu %q out of bounds: %+v", info.Name, info.Bounds)
		}
	}

	// Clicking unapplied tag applies one tag and closes entire menu.
	clickCenter(tt, longItem.Bounds)
	if len(tt.Menu()) != 0 || !slices.Contains(a.items[0].Tags, longTag) {
		t.Fatalf("expected tag %q applied and menu closed, tags=%q menu=%q", longTag, a.items[0].Tags, tt.Menu())
	}

	// Re-open Quick Tag and click already-applied 'work' tag -> removes 'work' and closes menu.
	tt.RightClickAt(panelWidth-40, rect.Y+rect.H/2)
	quickAgain, _ := tt.AccessByRole(ui.RoleMenuItem, msgs.CardMenuQuickTag())
	tt.Move(quickAgain.Bounds.X+quickAgain.Bounds.W/2, quickAgain.Bounds.Y+quickAgain.Bounds.H/2)
	workAgain, ok := tt.AccessByRole(ui.RoleMenuItem, "work")
	if !ok {
		t.Fatalf("expected 'work' before toggle-off, got ok=%v info=%+v", ok, workAgain)
	}
	clickCenter(tt, workAgain.Bounds)
	if len(tt.Menu()) != 0 || slices.Contains(a.items[0].Tags, "work") {
		t.Fatalf("expected tag 'work' removed and menu closed, tags=%q menu=%q", a.items[0].Tags, tt.Menu())
	}

	// Left & middle clicks must also keep submenu within [0, 400].
	for _, clickX := range []float32{24, 140, 200, 260} {
		tt.RightClickAt(clickX, rect.Y+rect.H/2)
		qItem, _ := tt.AccessByRole(ui.RoleMenuItem, msgs.CardMenuQuickTag())
		tt.Move(qItem.Bounds.X+qItem.Bounds.W/2, qItem.Bounds.Y+qItem.Bounds.H/2)
		subItem, ok := tt.AccessByRole(ui.RoleMenuItem, longTag)
		if !ok || subItem.Bounds.X < 0 || subItem.Bounds.X+subItem.Bounds.W > panelWidth {
			t.Fatalf("clickX=%.0f: submenu out of 400px panel: ok=%v sub=%+v", clickX, ok, subItem)
		}
		tt.Key(0, ui.KeyEscape)
		tt.Key(0, ui.KeyEscape)
	}
}

func TestShowAllCardActionsAndRichActionAlignment(t *testing.T) {
	writes := stubWriteClipItem(t)
	msgs := locales.In("zh-Hans")

	st, a := newTestAppWithStore(t)
	_, _ = st.SaveText("plain row", store.SaveOptions{})
	_, _ = st.SaveText("rich row", store.SaveOptions{HTML: "<p><b>rich</b> row</p>"})
	a.reload()
	if !a.settings.ShowAllCardActions {
		t.Fatalf("expected ShowAllCardActions default true")
	}

	tt := ui.NewTester(a.view, panelWidth, panelHeight)
	if err := tt.Click(msgs.RichMenuPastePlain()); err != nil {
		t.Fatalf("click rich card plain paste button: %v", err)
	}
	if len(*writes) != 1 || !(*writes)[0].plainText || (*writes)[0].item.Text != "rich row" {
		t.Fatalf("expected plain paste of rich row, writes=%+v", *writes)
	}

	var pinXs, delXs []float32
	for _, info := range tt.AccessAll() {
		if info.Role == ui.RoleButton && info.Name == msgs.CardActionPin() {
			pinXs = append(pinXs, info.Bounds.X)
		}
		if info.Role == ui.RoleButton && info.Name == msgs.CardActionDelete() {
			delXs = append(delXs, info.Bounds.X)
		}
	}
	if len(pinXs) != 2 || len(delXs) != 2 || pinXs[0] != pinXs[1] || delXs[0] != delXs[1] {
		t.Fatalf("expected aligned pin/delete X across card kinds, pin=%v del=%v", pinXs, delXs)
	}

	// Toggle ShowAllCardActions OFF in settings -> persisted, only Pin/Delete remain on card, context menu stays full.
	if err := tt.Click(msgs.HeaderSettings()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	if err := tt.Click(msgs.SettingsGroupAppearance()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	if err := tt.Click(msgs.SettingsSwitchA11y(msgs.SettingsShowAllCardActions())); err != nil {
		t.Fatal(err)
	}
	if loadedCfg, err := st.LoadSettings(); err != nil || loadedCfg.ShowAllCardActions {
		t.Fatalf("expected ShowAllCardActions=false persisted, err=%v cfg=%+v", err, loadedCfg)
	}
	if err := tt.Click(msgs.HeaderBack()); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)

	for _, unexpected := range []string{msgs.CardActionOpen(), msgs.CardActionTag(), msgs.RichMenuPastePlain()} {
		if _, ok := tt.AccessByRole(ui.RoleButton, unexpected); ok {
			t.Fatalf("expected button %q hidden when ShowAllCardActions=false", unexpected)
		}
	}
	if _, ok := tt.AccessByRole(ui.RoleButton, msgs.CardActionPin()); !ok {
		t.Fatalf("expected Pin button visible when ShowAllCardActions=false")
	}

	richRect, _ := tt.Find("rich row")
	tt.RightClickAt(richRect.X+16, richRect.Y+richRect.H/2)
	wantRich := []string{msgs.CardActionOpen(), msgs.RichMenuPastePlain(), msgs.CardMenuEditTags(), msgs.CardActionPin(), "-", msgs.CardActionDelete()}
	if got := tt.Menu(); !slices.Equal(got, wantRich) {
		t.Fatalf("expected full context menu %q, got %q", wantRich, got)
	}
}

func TestFileOpenAndHidePanel(t *testing.T) {
	opened, shown := stubShellOpenAndFolder(t)
	dir := t.TempDir()
	existing := filepath.Join(dir, "report.pdf")
	first := filepath.Join(dir, "01-first.txt")
	second := filepath.Join(dir, "02-second.txt")
	for _, p := range []string{existing, first, second} {
		if err := os.WriteFile(p, []byte("ok"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	missing := filepath.Join(dir, "gone.pdf")

	// Single existing file: open + hidePanel.
	a := newTestAppWithFiles(t, []string{existing})
	a.routedSearchActive = true
	tt := ui.NewTester(a.view, panelWidth, panelHeight)
	if err := tt.Click(locales.In("zh-Hans").CardActionOpen()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(*opened, []string{existing}) || len(*shown) != 0 || a.routedSearchActive {
		t.Fatalf("expected single file opened & panel hidden, opened=%q shown=%q active=%v", *opened, *shown, a.routedSearchActive)
	}

	// Missing file: toast shown, panel stays open.
	a2 := newTestAppWithFiles(t, []string{missing})
	a2.routedSearchActive = true
	a2.openItem(a2.items[0])
	if len(*opened) != 1 || a2.pendingToast != locales.StatusFileMissing() || !a2.routedSearchActive {
		t.Fatalf("expected missing file toast without hiding panel, toast=%q active=%v", a2.pendingToast, a2.routedSearchActive)
	}

	// Multi-file fallback: reveals first existing file & hides panel without opening all.
	a3 := newTestAppWithFiles(t, []string{missing, first, second})
	a3.routedSearchActive = true
	a3.openItem(a3.items[0])
	if len(*opened) != 1 || !slices.Equal(*shown, []string{first}) || a3.routedSearchActive {
		t.Fatalf("expected multi-file reveal %q & panel hidden, opened=%q shown=%q active=%v", first, *opened, *shown, a3.routedSearchActive)
	}
}

func TestMultiFilePopoverMenu(t *testing.T) {
	opened, shown := stubShellOpenAndFolder(t)
	msgs := locales.In("zh-Hans")

	t.Run("same directory missing and show in folder", func(t *testing.T) {
		*opened, *shown = nil, nil
		dir := t.TempDir()
		f1, f2 := filepath.Join(dir, "alpha.txt"), filepath.Join(dir, "beta.txt")
		for _, p := range []string{f1, f2} {
			_ = os.WriteFile(p, []byte("x"), 0o600)
		}
		a := newTestAppWithFiles(t, []string{f1, f2, filepath.Join(dir, "deleted.txt")})
		tt := ui.NewTester(a.view, panelWidth, panelHeight)

		if err := tt.Click(msgs.CardActionOpenFiles()); err != nil {
			t.Fatal(err)
		}
		if !a.hasOpenFileMenu() || len(*opened) != 0 {
			t.Fatalf("expected file menu open without opening files")
		}
		showInfo, ok := tt.AccessByRole(ui.RoleMenuItem, msgs.FileMenuShowInFolder())
		if !ok || showInfo.Disabled || showInfo.Bounds.W > fileMenuMultiMaxWidth || showInfo.Bounds.H > 32 {
			t.Fatalf("unexpected Show in Folder item: ok=%v info=%+v", ok, showInfo)
		}
		if delInfo, ok := tt.AccessByRole(ui.RoleMenuItem, "deleted.txt"); !ok || !delInfo.Disabled {
			t.Fatalf("expected deleted.txt disabled, got ok=%v info=%+v", ok, delInfo)
		}
		betaInfo, _ := tt.AccessByRole(ui.RoleMenuItem, "beta.txt")
		clickCenter(tt, betaInfo.Bounds)
		if !slices.Equal(*opened, []string{f2}) || a.hasOpenFileMenu() {
			t.Fatalf("expected beta.txt opened and menu closed, opened=%q", *opened)
		}

		_ = tt.Click(msgs.CardActionOpenFiles())
		showInfo, _ = tt.AccessByRole(ui.RoleMenuItem, msgs.FileMenuShowInFolder())
		clickCenter(tt, showInfo.Bounds)
		if !slices.Equal(*shown, []string{f1}) {
			t.Fatalf("expected ShowItemInFolder(%q), got %q", f1, *shown)
		}
	})

	t.Run("cross directory duplicates and excess limit", func(t *testing.T) {
		*opened, *shown = nil, nil
		root := t.TempDir()
		dirA, dirB := filepath.Join(root, "dirA"), filepath.Join(root, "dirB")
		_ = os.MkdirAll(dirA, 0o755)
		_ = os.MkdirAll(dirB, 0o755)
		readmeA, readmeB := filepath.Join(dirA, "readme.md"), filepath.Join(dirB, "readme.md")
		_ = os.WriteFile(readmeA, []byte("a"), 0o600)
		_ = os.WriteFile(readmeB, []byte("b"), 0o600)

		a := newTestAppWithFiles(t, []string{readmeA, readmeB})
		tt := ui.NewTester(a.view, panelWidth, panelHeight)
		_ = tt.Click(msgs.CardActionOpenFiles())
		var targetB ui.AccessInfo
		for _, info := range tt.AccessAll() {
			if info.Role == ui.RoleMenuItem && info.Name == "readme.md" && strings.Contains(info.Description, dirB) {
				targetB = info
			}
		}
		clickCenter(tt, targetB.Bounds)
		if !slices.Equal(*opened, []string{readmeB}) {
			t.Fatalf("expected dirB readme opened, got %q", *opened)
		}

		_ = tt.Click(msgs.CardActionOpenFiles())
		dirBItem, ok := tt.AccessByRole(ui.RoleMenuItem, msgs.FileMenuShowNamedFolder("dirB"))
		if !ok || dirBItem.Bounds.X < 0 || dirBItem.Bounds.X+dirBItem.Bounds.W > panelWidth {
			t.Fatalf("unexpected dirB item: ok=%v bounds=%+v", ok, dirBItem.Bounds)
		}
		clickCenter(tt, dirBItem.Bounds)
		if !slices.Equal(*shown, []string{readmeB}) {
			t.Fatalf("expected dirB reveal %q, got %q", readmeB, *shown)
		}

		// Excess (>50) files omitted with disabled summary item.
		many := make([]string, 55)
		for i := range many {
			many[i] = filepath.Join(root, fmt.Sprintf("item-%02d.txt", i+1))
		}
		aMany := newTestAppWithFiles(t, many)
		ttMany := ui.NewTester(aMany.view, panelWidth, panelHeight)
		_ = ttMany.Click(msgs.CardActionOpenFiles())
		if _, ok := ttMany.AccessByRole(ui.RoleMenuItem, "item-50.txt"); !ok {
			t.Fatalf("expected item-50.txt in menu")
		}
		if _, ok := ttMany.AccessByRole(ui.RoleMenuItem, "item-51.txt"); ok {
			t.Fatalf("expected item-51.txt omitted past maxFileMenuEntries")
		}
		if more, ok := ttMany.AccessByRole(ui.RoleMenuItem, msgs.FileMenuMoreOmitted(5)); !ok || !more.Disabled {
			t.Fatalf("expected disabled omitted notice, got ok=%v info=%+v", ok, more)
		}
	})
}

func TestFileMenuHideCardActionsAndNoFocusEscape(t *testing.T) {
	opened, _ := stubShellOpenAndFolder(t)
	msgs := locales.In("zh-Hans")
	dir := t.TempDir()
	f1, f2 := filepath.Join(dir, "one.txt"), filepath.Join(dir, "two.txt")
	_ = os.WriteFile(f1, []byte("v"), 0o600)
	_ = os.WriteFile(f2, []byte("v"), 0o600)

	a := newTestAppWithFiles(t, []string{f1, f2})
	a.settings.HideCardActions = true
	tt := ui.NewTester(a.view, panelWidth, panelHeight)

	cardRect, _ := tt.Find("one.txt")
	tt.Move(cardRect.X+20, cardRect.Y+cardRect.H/2)
	btnInfo, ok := tt.AccessByRole(ui.RoleMenuButton, msgs.CardActionOpenFiles())
	if !ok {
		t.Fatalf("expected file menu button visible while hovered")
	}
	clickCenter(tt, btnInfo.Bounds)
	tt.Move(10, panelHeight-10)
	if _, ok := tt.AccessByRole(ui.RoleMenuItem, "two.txt"); !ok {
		t.Fatalf("expected menu to remain open when pointer leaves card")
	}

	a.applyRoutedKey(sys.RoutedKey{VK: vkEscape})
	tt.Frame()
	if a.hasOpenFileMenu() || len(*opened) != 0 {
		t.Fatalf("expected Escape to close menu without opening files")
	}
}

func TestContextMenuDismissedOnHidePanelAndReopens(t *testing.T) {
	msgs := locales.In("zh-Hans")
	st, a := newTestAppWithStore(t)
	_, _ = st.SaveText("hide panel menu check", store.SaveOptions{})
	a.reload()

	tt := ui.NewTester(a.view, panelWidth, panelHeight)
	rect, ok := tt.Find("hide panel menu check")
	if !ok {
		t.Fatalf("expected card in view")
	}

	tt.RightClickAt(rect.X+16, rect.Y+rect.H/2)
	if got := tt.Menu(); len(got) == 0 {
		t.Fatalf("expected context menu open after right-click")
	}

	// Simulate hiding and re-showing the panel: menu must be dismissed on next frame.
	a.hidePanel()
	tt.Frame()
	if got := tt.Menu(); len(got) != 0 {
		t.Fatalf("expected context menu closed after hidePanel, got %q", got)
	}

	// Right-clicking again after re-show must still open the menu normally.
	tt.RightClickAt(rect.X+16, rect.Y+rect.H/2)
	want := []string{msgs.CardActionOpen(), msgs.CardMenuEditTags(), msgs.CardActionPin(), "-", msgs.CardActionDelete()}
	if got := tt.Menu(); !slices.Equal(got, want) {
		t.Fatalf("expected context menu %q to reopen normally, got %q", want, got)
	}
}
