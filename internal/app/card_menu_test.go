package app

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"mytiez/internal/store"
	"mytiez/locales"

	"github.com/egoist/mygo/ui"
)

func newTestAppWithStore(t *testing.T) (*store.Store, *clipApp) {
	t.Helper()
	st, err := store.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = st.Close()
	})
	a := newClipApp(st, nil)
	return st, a
}

func saveTestImage(t *testing.T, st *store.Store) store.ClipItem {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 12, 12))
	tmp := filepath.Join(t.TempDir(), "sample.png")
	f, err := os.Create(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
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

func TestUnifiedContextMenuVisibilityAndOrderAcrossAllCardKinds(t *testing.T) {
	msgs := locales.In("zh-Hans")
	dir := t.TempDir()
	singleFile := filepath.Join(dir, "single-doc.txt")
	multiA := filepath.Join(dir, "multi-a.txt")
	multiB := filepath.Join(dir, "multi-b.txt")
	for _, p := range []string{singleFile, multiA, multiB} {
		if err := os.WriteFile(p, []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("non-file kinds without global tags", func(t *testing.T) {
		st, a := newTestAppWithStore(t)
		if _, err := st.SaveText("plain text card", store.SaveOptions{}); err != nil {
			t.Fatal(err)
		}
		if _, err := st.SaveText("https://example.com/docs", store.SaveOptions{}); err != nil {
			t.Fatal(err)
		}
		if _, err := st.SaveText("func main() {\n\tfmt.Println(1)\n}", store.SaveOptions{}); err != nil {
			t.Fatal(err)
		}
		if _, err := st.SaveText("rich card", store.SaveOptions{HTML: "<p><b>rich</b> card</p>"}); err != nil {
			t.Fatal(err)
		}
		imgItem := saveTestImage(t, st)
		a.reload()

		tt := ui.NewTester(a.view, panelWidth, 900)

		cases := []struct {
			name     string
			findText string
			wantMenu []string
		}{
			{
				name:     "text",
				findText: "plain text card",
				wantMenu: []string{msgs.CardActionOpen(), msgs.CardMenuEditTags(), msgs.CardActionPin(), "-", msgs.CardActionDelete()},
			},
			{
				name:     "url",
				findText: "https://example.com/docs",
				wantMenu: []string{msgs.CardActionOpen(), msgs.CardMenuEditTags(), msgs.CardActionPin(), "-", msgs.CardActionDelete()},
			},
			{
				name:     "code",
				findText: "func main() { \tfmt.Println(1) }",
				wantMenu: []string{msgs.CardActionOpen(), msgs.CardMenuEditTags(), msgs.CardActionPin(), "-", msgs.CardActionDelete()},
			},
			{
				name:     "rich_text",
				findText: "rich card",
				wantMenu: []string{msgs.CardActionOpen(), msgs.RichMenuPastePlain(), msgs.CardMenuEditTags(), msgs.CardActionPin(), "-", msgs.CardActionDelete()},
			},
		}

		for _, tc := range cases {
			rect, ok := tt.Find(tc.findText)
			if !ok {
				t.Fatalf("%s: expected %q in view, texts=%q", tc.name, tc.findText, tt.Texts())
			}
			tt.RightClickAt(rect.X+16, rect.Y+rect.H/2)
			if got := tt.Menu(); !slices.Equal(got, tc.wantMenu) {
				t.Fatalf("%s: expected menu %q, got %q", tc.name, tc.wantMenu, got)
			}
			tt.Key(0, ui.KeyEscape)
		}

		// Image card: filter to image tab so we locate the image card reliably.
		if err := tt.Click(msgs.TabImage()); err != nil {
			t.Fatal(err)
		}
		imgAppName := "Unknown"
		if imgItem.SourceApp != "" {
			imgAppName = imgItem.SourceApp
		}
		rect, ok := tt.Find(imgAppName)
		if !ok {
			t.Fatalf("image: expected %q in image filter view, texts=%q", imgAppName, tt.Texts())
		}
		tt.RightClickAt(rect.X+16, rect.Y+28)
		wantImgMenu := []string{msgs.CardActionOpen(), msgs.CardMenuEditTags(), msgs.CardActionPin(), "-", msgs.CardActionDelete()}
		if got := tt.Menu(); !slices.Equal(got, wantImgMenu) {
			t.Fatalf("image: expected menu %q, got %q", wantImgMenu, got)
		}
		tt.Key(0, ui.KeyEscape)
	})

	t.Run("single and multi file kinds", func(t *testing.T) {
		st, a := newTestAppWithStore(t)
		if _, err := st.UpsertFiles([]string{singleFile}); err != nil {
			t.Fatal(err)
		}
		if _, err := st.UpsertFiles([]string{multiA, multiB}); err != nil {
			t.Fatal(err)
		}
		a.reload()

		tt := ui.NewTester(a.view, panelWidth, panelHeight)

		singleRect, ok := tt.Find("single-doc.txt")
		if !ok {
			t.Fatalf("expected single-doc.txt in view, texts=%q", tt.Texts())
		}
		tt.RightClickAt(singleRect.X+16, singleRect.Y+singleRect.H/2)
		wantSingle := []string{
			msgs.CardActionOpen(),
			msgs.FileMenuShowInFolder(),
			msgs.CardMenuEditTags(),
			msgs.CardActionPin(),
			"-",
			msgs.CardActionDelete(),
		}
		if got := tt.Menu(); !slices.Equal(got, wantSingle) {
			t.Fatalf("single file: expected menu %q, got %q", wantSingle, got)
		}
		tt.Key(0, ui.KeyEscape)

		multiRect, ok := tt.Find("multi-a.txt")
		if !ok {
			t.Fatalf("expected multi-a.txt in view, texts=%q", tt.Texts())
		}
		tt.RightClickAt(multiRect.X+16, multiRect.Y+multiRect.H/2)
		wantMulti := []string{
			msgs.CardActionOpen(),
			msgs.CardMenuEditTags(),
			msgs.CardActionPin(),
			"-",
			msgs.CardActionDelete(),
		}
		if got := tt.Menu(); !slices.Equal(got, wantMulti) {
			t.Fatalf("multi file: expected menu %q, got %q", wantMulti, got)
		}
		// Hover "Open" to confirm it expands the multi-file submenu.
		openSub, ok := tt.AccessByRole(ui.RoleMenuItem, msgs.CardActionOpen())
		if !ok {
			t.Fatalf("expected multi-file Open submenu item")
		}
		tt.Move(openSub.Bounds.X+openSub.Bounds.W/2, openSub.Bounds.Y+openSub.Bounds.H/2)
		if _, ok := tt.AccessByRole(ui.RoleMenuItem, "multi-a.txt"); !ok {
			t.Fatalf("expected multi-a.txt inside multi-file Open submenu")
		}
		tt.Key(0, ui.KeyEscape)
		tt.Key(0, ui.KeyEscape)
	})
}

func TestUnifiedContextMenuActionsOpenEditTagsPinDelete(t *testing.T) {
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

	// 1. Open
	tt.RightClickAt(rect.X+16, rect.Y+rect.H/2)
	openInfo, ok := tt.AccessByRole(ui.RoleMenuItem, msgs.CardActionOpen())
	if !ok {
		t.Fatalf("expected open menu item, menu=%q", tt.Menu())
	}
	clickCenter(tt, openInfo.Bounds)
	wantName := fmt.Sprintf("MyTieZ_entry_%d.txt", item.ID)
	if len(*opened) != 1 || filepath.Base((*opened)[0]) != wantName {
		t.Fatalf("expected 1 open path call with %q, got %q", wantName, *opened)
	}

	// 2. Edit Tags opens the card-level tag editor and closes the menu.
	tt.RightClickAt(rect.X+16, rect.Y+rect.H/2)
	editInfo, ok := tt.AccessByRole(ui.RoleMenuItem, msgs.CardMenuEditTags())
	if !ok {
		t.Fatalf("expected edit tags menu item, menu=%q", tt.Menu())
	}
	clickCenter(tt, editInfo.Bounds)
	if a.editingTagsID != item.ID {
		t.Fatalf("expected editingTagsID=%d after choosing Edit Tags, got %d", item.ID, a.editingTagsID)
	}
	if len(tt.Menu()) != 0 {
		t.Fatalf("expected menu closed after Edit Tags, got %q", tt.Menu())
	}
	if _, ok := tt.AccessByRole(ui.RoleTextField, msgs.TagsInputLabel()); !ok {
		t.Fatalf("expected card tag input visible after Edit Tags")
	}
	// Close editor via Done button.
	if err := tt.Click(msgs.TagsDoneA11y()); err != nil {
		t.Fatal(err)
	}

	// 3. Pin then Unpin
	tt.RightClickAt(rect.X+16, rect.Y+rect.H/2)
	pinInfo, ok := tt.AccessByRole(ui.RoleMenuItem, msgs.CardActionPin())
	if !ok {
		t.Fatalf("expected Pin menu item, menu=%q", tt.Menu())
	}
	clickCenter(tt, pinInfo.Bounds)
	if !a.items[0].Pinned {
		t.Fatalf("expected item pinned after clicking Pin")
	}

	tt.RightClickAt(rect.X+16, rect.Y+rect.H/2)
	unpinInfo, ok := tt.AccessByRole(ui.RoleMenuItem, msgs.CardMenuUnpin())
	if !ok {
		t.Fatalf("expected Unpin menu item when pinned, menu=%q", tt.Menu())
	}
	clickCenter(tt, unpinInfo.Bounds)
	if a.items[0].Pinned {
		t.Fatalf("expected item unpinned after clicking Unpin")
	}

	// 4. Delete
	tt.RightClickAt(rect.X+16, rect.Y+rect.H/2)
	delInfo, ok := tt.AccessByRole(ui.RoleMenuItem, msgs.CardActionDelete())
	if !ok {
		t.Fatalf("expected Delete menu item, menu=%q", tt.Menu())
	}
	clickCenter(tt, delInfo.Bounds)
	if len(a.items) != 0 {
		t.Fatalf("expected item deleted, remaining=%d", len(a.items))
	}
}

func TestQuickTagSubmenuAndLeftFlipWithin400pxPanel(t *testing.T) {
	msgs := locales.In("zh-Hans")
	st, a := newTestAppWithStore(t)

	const longTag = "very-long-tag-name-that-should-be-constrained-inside-submenu-width"
	taggedItem, err := st.SaveText("already tagged seed", store.SaveOptions{Tags: []string{"work", longTag}})
	if err != nil {
		t.Fatal(err)
	}
	targetItem, err := st.SaveText("target card for quick tag", store.SaveOptions{Tags: []string{"work"}})
	if err != nil {
		t.Fatal(err)
	}
	_ = taggedItem
	a.reload()

	tt := ui.NewTester(a.view, panelWidth, panelHeight)
	rect, ok := tt.Find("target card for quick tag")
	if !ok {
		t.Fatalf("expected target card in view")
	}

	// Right-click near the right edge of the 400px panel so the submenu has no
	// room on the right and must flip to the left of the primary menu.
	tt.RightClickAt(panelWidth-40, rect.Y+rect.H/2)

	wantMenu := []string{
		msgs.CardActionOpen(),
		msgs.CardMenuEditTags(),
		msgs.CardMenuQuickTag(),
		msgs.CardActionPin(),
		"-",
		msgs.CardActionDelete(),
	}
	if got := tt.Menu(); !slices.Equal(got, wantMenu) {
		t.Fatalf("expected menu %q, got %q", wantMenu, got)
	}

	quickInfo, ok := tt.AccessByRole(ui.RoleMenuItem, msgs.CardMenuQuickTag())
	if !ok {
		t.Fatalf("expected Quick Tag submenu trigger")
	}
	// Hover Quick Tag to open submenu.
	tt.Move(quickInfo.Bounds.X+quickInfo.Bounds.W/2, quickInfo.Bounds.Y+quickInfo.Bounds.H/2)

	workItem, ok := tt.AccessByRole(ui.RoleMenuItem, "work")
	if !ok || !workItem.Disabled {
		t.Fatalf("expected already-applied tag 'work' disabled in submenu, got ok=%v info=%+v", ok, workItem)
	}
	longItem, ok := tt.AccessByRole(ui.RoleMenuItem, longTag)
	if !ok || longItem.Disabled {
		t.Fatalf("expected unapplied long tag enabled in submenu, got ok=%v info=%+v", ok, longItem)
	}

	// Verify submenu stayed within the 400px panel and flipped left when needed.
	for _, info := range []ui.AccessInfo{workItem, longItem} {
		if info.Bounds.X < 0 || info.Bounds.X+info.Bounds.W > panelWidth {
			t.Fatalf("expected submenu item %q inside 400px panel, got bounds=%+v (trigger=%+v)", info.Name, info.Bounds, quickInfo.Bounds)
		}
		if info.Bounds.W > cardMenuMaxWidth {
			t.Fatalf("expected submenu item %q width <= %.0f, got bounds=%+v", info.Name, cardMenuMaxWidth, info.Bounds)
		}
	}
	if longItem.Bounds.X >= quickInfo.Bounds.X {
		t.Fatalf("expected submenu to flip left when opened near right edge: subBounds=%+v triggerBounds=%+v", longItem.Bounds, quickInfo.Bounds)
	}

	// Click the unapplied tag: applies one tag and closes the whole menu.
	clickCenter(tt, longItem.Bounds)
	if len(tt.Menu()) != 0 {
		t.Fatalf("expected entire menu closed after quick-tagging, menu=%q", tt.Menu())
	}
	var updated store.ClipItem
	for _, it := range a.items {
		if it.ID == targetItem.ID {
			updated = it
			break
		}
	}
	if !slices.Contains(updated.Tags, longTag) {
		t.Fatalf("expected target item to gain tag %q, got tags=%q", longTag, updated.Tags)
	}

	// Also verify opening near the left edge opens submenu to the right within bounds.
	tt.RightClickAt(24, rect.Y+rect.H/2)
	quickLeft, ok := tt.AccessByRole(ui.RoleMenuItem, msgs.CardMenuQuickTag())
	if !ok {
		t.Fatalf("expected Quick Tag submenu trigger on left-side click")
	}
	tt.Move(quickLeft.Bounds.X+quickLeft.Bounds.W/2, quickLeft.Bounds.Y+quickLeft.Bounds.H/2)
	subRight, ok := tt.AccessByRole(ui.RoleMenuItem, "work")
	if !ok {
		t.Fatalf("expected work tag in submenu on left-side click")
	}
	if subRight.Bounds.X <= quickLeft.Bounds.X || subRight.Bounds.X+subRight.Bounds.W > panelWidth {
		t.Fatalf("expected submenu to open right inside 400px panel: sub=%+v trigger=%+v", subRight.Bounds, quickLeft.Bounds)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Key(0, ui.KeyEscape)

	// Clicking near the horizontal middle must also keep the submenu within [0, 400].
	for _, clickX := range []float32{140, 200, 260} {
		tt.RightClickAt(clickX, rect.Y+rect.H/2)
		qItem, ok := tt.AccessByRole(ui.RoleMenuItem, msgs.CardMenuQuickTag())
		if !ok {
			t.Fatalf("clickX=%.0f: expected Quick Tag submenu trigger", clickX)
		}
		tt.Move(qItem.Bounds.X+qItem.Bounds.W/2, qItem.Bounds.Y+qItem.Bounds.H/2)
		subItem, ok := tt.AccessByRole(ui.RoleMenuItem, longTag)
		if !ok {
			t.Fatalf("clickX=%.0f: expected submenu item visible", clickX)
		}
		if subItem.Bounds.X < 0 || subItem.Bounds.X+subItem.Bounds.W > panelWidth {
			t.Fatalf("clickX=%.0f: submenu clipped outside 400px panel: sub=%+v trigger=%+v", clickX, subItem.Bounds, qItem.Bounds)
		}
		tt.Key(0, ui.KeyEscape)
		tt.Key(0, ui.KeyEscape)
	}
}

func TestShowAllCardActionsSettingAndRichCardActionAlignment(t *testing.T) {
	writes := stubWriteClipItem(t)
	msgs := locales.In("zh-Hans")

	st, a := newTestAppWithStore(t)
	if _, err := st.SaveText("plain row", store.SaveOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveText("rich row", store.SaveOptions{HTML: "<p><b>rich</b> row</p>"}); err != nil {
		t.Fatal(err)
	}
	a.reload()

	if !a.settings.ShowAllCardActions {
		t.Fatalf("expected ShowAllCardActions default true")
	}

	tt := ui.NewTester(a.view, panelWidth, panelHeight)

	// 1. Default ON: rich card shows plain-paste button and aligns pin/delete with plain card.
	if err := tt.Click(msgs.RichMenuPastePlain()); err != nil {
		t.Fatalf("click rich card plain paste action button: %v", err)
	}
	if len(*writes) != 1 || !(*writes)[0].plainText || (*writes)[0].item.Text != "rich row" {
		t.Fatalf("expected plain paste button to paste rich row as plain text, writes=%+v", *writes)
	}

	var pinButtons, deleteButtons []ui.AccessInfo
	for _, info := range tt.AccessAll() {
		if info.Role != ui.RoleButton {
			continue
		}
		switch info.Name {
		case msgs.CardActionPin():
			pinButtons = append(pinButtons, info)
		case msgs.CardActionDelete():
			deleteButtons = append(deleteButtons, info)
		}
	}
	if len(pinButtons) != 2 || len(deleteButtons) != 2 {
		t.Fatalf("expected 2 pin and 2 delete buttons, got pin=%d delete=%d", len(pinButtons), len(deleteButtons))
	}
	if pinButtons[0].Bounds.X != pinButtons[1].Bounds.X {
		t.Fatalf("expected pin button X aligned across card kinds: %v vs %v", pinButtons[0].Bounds, pinButtons[1].Bounds)
	}
	if deleteButtons[0].Bounds.X != deleteButtons[1].Bounds.X {
		t.Fatalf("expected delete button X aligned across card kinds: %v vs %v", deleteButtons[0].Bounds, deleteButtons[1].Bounds)
	}

	// 2. Toggle ShowAllCardActions OFF in settings -> persists and leaves only Pin & Delete on cards.
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
	if a.settings.ShowAllCardActions {
		t.Fatalf("expected ShowAllCardActions toggled off")
	}
	// Verify persisted to DB.
	loadedCfg, err := st.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if loadedCfg.ShowAllCardActions {
		t.Fatalf("expected ShowAllCardActions=false persisted in DB")
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
	if _, ok := tt.AccessByRole(ui.RoleButton, msgs.CardActionDelete()); !ok {
		t.Fatalf("expected Delete button visible when ShowAllCardActions=false")
	}

	// Context menu must still contain all actions when ShowAllCardActions=false.
	richRect, ok := tt.Find("rich row")
	if !ok {
		t.Fatalf("expected rich row in view")
	}
	tt.RightClickAt(richRect.X+16, richRect.Y+richRect.H/2)
	wantRichMenu := []string{msgs.CardActionOpen(), msgs.RichMenuPastePlain(), msgs.CardMenuEditTags(), msgs.CardActionPin(), "-", msgs.CardActionDelete()}
	if got := tt.Menu(); !slices.Equal(got, wantRichMenu) {
		t.Fatalf("expected full context menu %q even when ShowAllCardActions=false, got %q", wantRichMenu, got)
	}
}
