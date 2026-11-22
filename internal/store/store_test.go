package store

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func makeTestPNG(t *testing.T, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, c)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestStoreLifecycle(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	txt1, err := st.UpsertText("hello world")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)

	pngBytes := makeTestPNG(t, color.RGBA{R: 255, A: 255})
	imgItem, err := st.UpsertImage(pngBytes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(imgItem.ImagePath); err != nil {
		t.Fatalf("expected image file at %s: %v", imgItem.ImagePath, err)
	}
	time.Sleep(5 * time.Millisecond)

	fileItem, err := st.UpsertFiles([]string{filepath.Join("C:\\", "tmp", "demo.txt")})
	if err != nil {
		t.Fatal(err)
	}

	// 去重并顶置
	time.Sleep(5 * time.Millisecond)
	txt1Again, err := st.UpsertText("hello world")
	if err != nil {
		t.Fatal(err)
	}
	if txt1Again.ID != txt1.ID {
		t.Fatalf("expected dedup ID %d, got %d", txt1.ID, txt1Again.ID)
	}

	all, err := st.List("", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 || all[0].ID != txt1.ID {
		t.Fatalf("expected 3 items with deduped text first, got %+v", all)
	}

	// 置顶图片后应排在最前
	pinned, err := st.TogglePin(imgItem.ID)
	if err != nil || !pinned {
		t.Fatalf("expected pinned=true, got %v, err=%v", pinned, err)
	}
	all, err = st.List("", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if all[0].ID != imgItem.ID {
		t.Fatalf("expected pinned image first, got %+v", all[0])
	}

	// 按分类和关键词查询
	filesOnly, err := st.List("demo", KindFile, 50)
	if err != nil || len(filesOnly) != 1 || filesOnly[0].ID != fileItem.ID {
		t.Fatalf("expected 1 file item, got %+v, err=%v", filesOnly, err)
	}

	// 清空未置顶：只剩置顶的 imgItem
	if err := st.ClearUnpinned(); err != nil {
		t.Fatal(err)
	}
	remaining, err := st.List("", "", 50)
	if err != nil || len(remaining) != 1 || remaining[0].ID != imgItem.ID {
		t.Fatalf("expected only pinned image left, got %+v, err=%v", remaining, err)
	}

	// 删除最后一条图片记录应清理缓存图片文件
	if err := st.Delete(imgItem.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(imgItem.ImagePath); !os.IsNotExist(err) {
		t.Fatalf("expected orphan image file removed, err=%v", err)
	}
}

func TestStoreSettingsAndLimits(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cfg, err := st.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Deduplicate || cfg.MaxHistory != 500 || cfg.Hotkey != "Alt+V" || cfg.NoFocusMode || !cfg.FollowMouse {
		t.Fatalf("unexpected default settings: %+v", cfg)
	}

	cfg.Deduplicate = false
	cfg.CompactMode = true
	cfg.NoFocusMode = true
	cfg.ColorMode = "dark"
	for _, mode := range []string{"mica", "mica_alt", "acrylic_base", "acrylic"} {
		cfg.BackdropMode = mode
		if err := st.SaveSettings(cfg); err != nil {
			t.Fatal(err)
		}
		loadedMode, err := st.LoadSettings()
		if err != nil || loadedMode.BackdropMode != mode {
			t.Fatalf("expected BackdropMode=%q, got %q err=%v", mode, loadedMode.BackdropMode, err)
		}
	}
	cfg.BackdropMode = "invalid_mode"
	if err := st.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	fallbackCfg, err := st.LoadSettings()
	if err != nil || fallbackCfg.BackdropMode != "solid" {
		t.Fatalf("expected invalid BackdropMode fallback to solid, got %q err=%v", fallbackCfg.BackdropMode, err)
	}
	cfg.BackdropMode = "solid"
	if err := st.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := st.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Deduplicate || !loaded.CompactMode || !loaded.NoFocusMode || loaded.ColorMode != "dark" {
		t.Fatalf("unexpected loaded settings: %+v", loaded)
	}

	// 兼容旧字段 hotkey_no_focus 自动迁移到 no_focus_mode
	if _, err := st.db.Exec(`UPDATE app_settings SET value = '{"hotkey_no_focus":true}' WHERE key = 'config'`); err != nil {
		t.Fatal(err)
	}
	migrated, err := st.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !migrated.NoFocusMode {
		t.Fatalf("expected legacy hotkey_no_focus migrated to NoFocusMode=true, got %+v", migrated)
	}
	if err := st.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}

	// 关闭去重时允许同内容重复入库
	if _, err := st.SaveText("dup", SaveOptions{Deduplicate: false}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveText("dup", SaveOptions{Deduplicate: false}); err != nil {
		t.Fatal(err)
	}
	items, err := st.List("", "", 50)
	if err != nil || len(items) != 2 {
		t.Fatalf("expected 2 items without dedup, got %d, err=%v", len(items), err)
	}

	// 置顶 1 条，并测试 MaxHistory=1 只保留 1 条未置顶 + 所有置顶
	if _, err := st.TogglePin(items[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveText("third", SaveOptions{Deduplicate: true, MaxHistory: 1}); err != nil {
		t.Fatal(err)
	}
	items, err = st.List("", "", 50)
	if err != nil || len(items) != 2 {
		t.Fatalf("expected 1 pinned + 1 unpinned (2 total), got %d (%+v), err=%v", len(items), items, err)
	}

	// 清空全部应连置顶一起删除
	if err := st.ClearAll(); err != nil {
		t.Fatal(err)
	}
	items, err = st.List("", "", 50)
	if err != nil || len(items) != 0 {
		t.Fatalf("expected 0 items after ClearAll, got %d, err=%v", len(items), err)
	}
}

func TestWindowSizeSettingsFallback(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cfg := DefaultSettings()
	cfg.WindowWidth = 512
	cfg.WindowHeight = 720
	if err := st.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := st.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.WindowWidth != 512 || loaded.WindowHeight != 720 {
		t.Fatalf("expected persisted window size 512x720, got %dx%d", loaded.WindowWidth, loaded.WindowHeight)
	}

	// 低于最小尺寸时自动回退默认尺寸
	cfg.WindowWidth = 100
	cfg.WindowHeight = 100
	if err := st.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err = st.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.WindowWidth != 400 || loaded.WindowHeight != 660 {
		t.Fatalf("expected fallback window size 400x660, got %dx%d", loaded.WindowWidth, loaded.WindowHeight)
	}
}

func TestSaveRichTextHTML(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	plain, err := st.SaveText("Hello", SaveOptions{Deduplicate: true})
	if err != nil {
		t.Fatal(err)
	}
	rich, err := st.SaveText("Hello", SaveOptions{
		Deduplicate: true,
		HTML:        "<b>Hello</b>",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rich.ID == plain.ID || rich.Kind != KindRichText || rich.HTML != "<b>Hello</b>" {
		t.Fatalf("expected separate rich_text item with HTML preserved, got id=%d (plain=%d), kind=%q, html=%q", rich.ID, plain.ID, rich.Kind, rich.HTML)
	}

	items, err := st.List("", KindRichText, 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("expected 1 rich_text item, got %d, err=%v", len(items), err)
	}
	if items[0].HTML != "<b>Hello</b>" || items[0].Preview != "Hello" {
		t.Fatalf("unexpected rich item fields: html=%q preview=%q", items[0].HTML, items[0].Preview)
	}
}

func TestTagsCRUDAndFilter(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	a, err := st.UpsertText("alpha snippet")
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.UpsertText("beta snippet")
	if err != nil {
		t.Fatal(err)
	}

	tags, err := st.UpdateTags(a.ID, []string{" 工作 ", "代码", "工作", ""})
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 || tags[0] != "工作" || tags[1] != "代码" {
		t.Fatalf("unexpected normalized tags: %#v", tags)
	}
	if _, err := st.UpdateTags(b.ID, []string{"工作"}); err != nil {
		t.Fatal(err)
	}

	allTags, err := st.ListAllTags()
	if err != nil {
		t.Fatal(err)
	}
	if len(allTags) != 2 || allTags[0] != "工作" || allTags[1] != "代码" {
		t.Fatalf("expected allTags ordered by frequency [工作 代码], got %#v", allTags)
	}

	got, err := st.List("#代码", "", 10)
	if err != nil || len(got) != 1 || got[0].ID != a.ID {
		t.Fatalf("expected #代码 filter to return item a only, got %+v err=%v", got, err)
	}
	got, err = st.List("#工作 beta", "", 10)
	if err != nil || len(got) != 1 || got[0].ID != b.ID {
		t.Fatalf("expected #工作 beta filter to return item b only, got %+v err=%v", got, err)
	}
}

func TestDetectTextKind(t *testing.T) {
	cases := []struct {
		text string
		want ClipKind
	}{
		{"https://github.com/egoist/mygo", KindURL},
		{"www.example.com/docs", KindURL},
		{"func main() {\n\treturn 1;\n}", KindCode},
		{`{"name": "mytiez", "version": 1}`, KindCode},
		{"<div>hello</div>", KindCode},
		{"今天下午三点开会讨论需求", KindText},
		{"访问 https://example.com 查看详情", KindText},
	}
	for _, tc := range cases {
		if got := DetectTextKind(tc.text); got != tc.want {
			t.Errorf("DetectTextKind(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}

	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	u, err := st.UpsertText("https://example.org")
	if err != nil || u.Kind != KindURL {
		t.Fatalf("expected KindURL, got %+v err=%v", u, err)
	}
	c, err := st.UpsertText("package main\nimport \"fmt\"\nfunc main() {}")
	if err != nil || c.Kind != KindCode {
		t.Fatalf("expected KindCode, got %+v err=%v", c, err)
	}
}

func TestListSummariesAndGetByID(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	longText := string(bytes.Repeat([]byte("a"), 2048))
	if _, err := st.UpsertText(longText); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertText("short"); err != nil {
		t.Fatal(err)
	}

	summaries, err := st.ListSummaries("", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}
	if summaries[0].Text != "short" {
		t.Fatalf("expected short text kept in summary, got %q", summaries[0].Text)
	}
	if summaries[1].Text != "" || summaries[1].Preview == "" {
		t.Fatalf("expected large text omitted in summary with preview kept, got text=%d preview=%d", len(summaries[1].Text), len(summaries[1].Preview))
	}

	full, err := st.GetByID(summaries[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if full.Text != longText {
		t.Fatalf("expected full text length %d, got %d", len(longText), len(full.Text))
	}
}

func TestPinyinSearch(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	for _, txt := range []string{"微信聊天记录", "重庆火锅", "Go语言入门", "plain english"} {
		if _, err := st.UpsertText(txt); err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct {
		query string
		want  string
	}{
		{"weixin", "微信聊天记录"},
		{"wx", "微信聊天记录"},
		{"weix", "微信聊天记录"},
		{"chongqing", "重庆火锅"},
		{"cq", "重庆火锅"},
		{"goyy", "Go语言入门"},
		{"goyuyan", "Go语言入门"},
		{"english", "plain english"},
	}
	for _, tc := range cases {
		items, err := st.ListSummaries(tc.query, "", 10)
		if err != nil {
			t.Fatalf("query %q failed: %v", tc.query, err)
		}
		if len(items) == 0 || items[0].Text != tc.want {
			t.Fatalf("query %q: expected %q, got %+v", tc.query, tc.want, items)
		}
	}
}

func TestPrivacyDetectionAndMasking(t *testing.T) {
	kinds := DefaultPrivacyKinds()
	if !ContainsSensitiveInfo("13800138000", kinds, "") {
		t.Fatal("expected phone detected")
	}
	if !ContainsSensitiveInfo("11010519900307123X", kinds, "") {
		t.Fatal("expected idcard detected")
	}
	if !ContainsSensitiveInfo("alice@example.com", kinds, "") {
		t.Fatal("expected email detected")
	}
	if !ContainsSensitiveInfo("sk-1234567890abcdefghijklmnop", kinds, "") {
		t.Fatal("expected secret detected")
	}
	if ContainsSensitiveInfo("https://example.com", kinds, "") {
		t.Fatal("url should not be detected unless enabled")
	}
	if !ContainsSensitiveInfo("https://example.com", []string{"url"}, "") {
		t.Fatal("expected url detected when enabled")
	}
	if !ContainsSensitiveInfo("Abc123!@#", []string{"password"}, "") {
		t.Fatal("expected strong password detected")
	}
	if !ContainsSensitiveInfo("CUSTOM-998877", nil, "^CUSTOM-\\d+$") {
		t.Fatal("expected custom regex detected")
	}
	if ContainsSensitiveInfo("data:image/png;base64,13800138000", AllPrivacyKinds, "") {
		t.Fatal("data URL should be ignored")
	}

	if !HasSensitiveTag([]string{"密码"}) || !HasSensitiveTag([]string{"Sensitive"}) {
		t.Fatal("expected HasSensitiveTag true for sensitive/密码")
	}

	if got := MaskSensitiveText("13800138000", 3, 4, false); got != "138••••••8000" {
		t.Fatalf("unexpected masked phone: %q", got)
	}
	if got := MaskSensitiveText("alice@example.com", 2, 2, false); got != "al••••••@example.com" {
		t.Fatalf("unexpected masked email keep domain: %q", got)
	}
	if got := MaskSensitiveText("alice@example.com", 2, 2, true); got != "al••••••@•••.com" {
		t.Fatalf("unexpected masked email mask domain: %q", got)
	}
	if got := MaskSensitiveText("Ab1!", 3, 3, false); got != "••••••••" {
		t.Fatalf("expected short secret fully masked, got %q", got)
	}

	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	it, err := st.SaveText("13800138000", SaveOptions{Deduplicate: true, Tags: []string{SensitiveTag}})
	if err != nil {
		t.Fatal(err)
	}
	if !HasSensitiveTag(it.Tags) {
		t.Fatalf("expected saved item tagged sensitive, got %+v", it.Tags)
	}
}

func TestListSummariesRichTextAndLargePinyin(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	richHTML := `<b>重点</b>内容`
	if _, err := st.SaveText("重点内容", SaveOptions{HTML: richHTML}); err != nil {
		t.Fatal(err)
	}
	summaries, err := st.ListSummaries("", KindRichText, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 || summaries[0].HTML != richHTML {
		t.Fatalf("expected rich_text summary HTML %q, got %+v", richHTML, summaries)
	}

	largeText := strings.Repeat("a", 1100) + " 微信支付"
	if _, err := st.SaveText(largeText, SaveOptions{}); err != nil {
		t.Fatal(err)
	}
	hits, err := st.ListSummaries("wxzf", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Kind != KindText {
		t.Fatalf("expected pinyin search to match large text, got %+v", hits)
	}
	if hits[0].Text != "" {
		t.Fatalf("expected large text cleared in summary result, got len=%d", len(hits[0].Text))
	}
}

func TestConcurrentSaveDeduplicate(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	const workers = 24
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, workers*2)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := st.SaveText("快速Ctrl+C内容", SaveOptions{
				Deduplicate: true,
				HTML:        "<b>快速Ctrl+C内容</b>",
				SourceApp:   "zen.exe",
			}); err != nil {
				errs <- err
			}
		}()
	}
	pngData := makeTestPNG(t, color.RGBA{R: 12, G: 34, B: 56, A: 255})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := st.SaveImage(pngData, SaveOptions{
				Deduplicate: true,
				SourceApp:   "zen.exe",
			}); err != nil {
				errs <- err
			}
		}()
	}

	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	items, err := st.List("", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected exactly 2 deduplicated items (1 rich_text + 1 image), got %d: %+v", len(items), items)
	}
}

func TestSensitiveEncryptionLifecycle(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DPAPI encryption is Windows-only")
	}
	dir := t.TempDir()
	st, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	secretText := "sk-live-secret-99887766"
	secretHTML := "<code>sk-live-secret-99887766</code>"
	it, err := st.SaveText(secretText, SaveOptions{
		Deduplicate: true,
		HTML:        secretHTML,
		Tags:        []string{SensitiveTag},
	})
	if err != nil {
		t.Fatal(err)
	}
	if it.Text != secretText || it.HTML != secretHTML {
		t.Fatalf("expected returned item decrypted, got text=%q html=%q", it.Text, it.HTML)
	}

	var rawText, rawHTML, rawPreview string
	if err := st.db.QueryRow(`SELECT text_content, html_content, preview FROM clip_items WHERE id = ?`, it.ID).Scan(&rawText, &rawHTML, &rawPreview); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(rawText, EncryptPrefix) || !strings.HasPrefix(rawHTML, EncryptPrefix) || !strings.HasPrefix(rawPreview, EncryptPrefix) {
		t.Fatalf("expected raw DB fields encrypted with %q, got text=%q html=%q preview=%q", EncryptPrefix, rawText, rawHTML, rawPreview)
	}
	if strings.Contains(rawText, secretText) || strings.Contains(rawPreview, secretText) {
		t.Fatal("raw DB fields must not contain plaintext")
	}

	got, err := st.GetByID(it.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != secretText || got.HTML != secretHTML || got.Preview != secretText {
		t.Fatalf("expected GetByID decrypted, got %+v", got)
	}

	// Plain keyword & ASCII search should both match encrypted sensitive items.
	hits, err := st.ListSummaries("998877", "", 20)
	if err != nil || len(hits) != 1 || hits[0].ID != it.ID {
		t.Fatalf("expected ASCII search to hit encrypted item, hits=%+v err=%v", hits, err)
	}
	hitsCN, err := st.ListSummaries("sk-live", "", 20)
	if err != nil || len(hitsCN) != 1 || hitsCN[0].ID != it.ID {
		t.Fatalf("expected keyword search to hit encrypted item, hits=%+v err=%v", hitsCN, err)
	}

	// Removing sensitive tag should decrypt fields in DB.
	if _, err := st.UpdateTags(it.ID, []string{"work"}); err != nil {
		t.Fatal(err)
	}
	if err := st.db.QueryRow(`SELECT text_content, html_content, preview FROM clip_items WHERE id = ?`, it.ID).Scan(&rawText, &rawHTML, &rawPreview); err != nil {
		t.Fatal(err)
	}
	if rawText != secretText || rawHTML != secretHTML || rawPreview != secretText {
		t.Fatalf("expected plaintext after removing sensitive tag, got text=%q html=%q preview=%q", rawText, rawHTML, rawPreview)
	}

	// Adding sensitive tag back should re-encrypt fields in DB.
	if _, err := st.UpdateTags(it.ID, []string{"work", SensitiveTag}); err != nil {
		t.Fatal(err)
	}
	if err := st.db.QueryRow(`SELECT text_content, html_content, preview FROM clip_items WHERE id = ?`, it.ID).Scan(&rawText, &rawHTML, &rawPreview); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(rawText, EncryptPrefix) || !strings.HasPrefix(rawHTML, EncryptPrefix) || !strings.HasPrefix(rawPreview, EncryptPrefix) {
		t.Fatalf("expected re-encrypted fields after adding sensitive tag, got text=%q", rawText)
	}

	// Simulate legacy plaintext sensitive row and verify OpenStore aligns it.
	legacy, err := st.SaveText("legacy-secret-1234", SaveOptions{Tags: []string{"normal"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`UPDATE clip_items SET tags_json = '["sensitive"]' WHERE id = ?`, legacy.ID); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	st2, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()

	if err := st2.db.QueryRow(`SELECT text_content, preview FROM clip_items WHERE id = ?`, legacy.ID).Scan(&rawText, &rawPreview); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(rawText, EncryptPrefix) || !strings.HasPrefix(rawPreview, EncryptPrefix) {
		t.Fatalf("expected OpenStore to encrypt legacy sensitive row, got text=%q preview=%q", rawText, rawPreview)
	}
	aligned, err := st2.GetByID(legacy.ID)
	if err != nil || aligned.Text != "legacy-secret-1234" {
		t.Fatalf("expected aligned item decrypted on read, got %+v err=%v", aligned, err)
	}
}

func TestStoreSearchTagExactAndBeyond1000(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// Verify UpdateTags on missing ID returns sql.ErrNoRows.
	if _, err := st.UpdateTags(999999, []string{"work"}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows for missing ID, got %v", err)
	}

	// Verify SQL wildcard chars in tags are matched literally via json_each.
	it1, err := st.SaveText("tag literal item", SaveOptions{Tags: []string{"work_1", "100%"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveText("tag similar item", SaveOptions{Tags: []string{"workA1", "1000"}}); err != nil {
		t.Fatal(err)
	}
	hits, err := st.ListSummaries("#work_1", "", 10)
	if err != nil || len(hits) != 1 || hits[0].ID != it1.ID {
		t.Fatalf("expected exact #work_1 tag match, got %+v err=%v", hits, err)
	}
	hitsPct, err := st.ListSummaries("#100%", "", 10)
	if err != nil || len(hitsPct) != 1 || hitsPct[0].ID != it1.ID {
		t.Fatalf("expected exact #100%% tag match, got %+v err=%v", hitsPct, err)
	}

	// Insert > 1000 newer items and ensure oldest item remains searchable.
	target, err := st.SaveText("古老的微信账单记录", SaveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := st.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli() + 10000
	for i := 0; i < 1050; i++ {
		txt := fmt.Sprintf("filler entry %d", i)
		if _, err := tx.Exec(`
INSERT INTO clip_items (kind, content_hash, text_content, html_content, image_path, files_json, preview, source_app, source_path, byte_size, pinned, tags_json, created_at, updated_at)
VALUES ('text', ?, ?, '', '', '[]', ?, 'test', '', 16, 0, '[]', ?, ?)
`, hashBytes([]byte(txt)), txt, txt, now+int64(i), now+int64(i)); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	pyHits, err := st.ListSummaries("wxzd", "", 20)
	if err != nil || len(pyHits) != 1 || pyHits[0].ID != target.ID {
		t.Fatalf("expected pinyin search to find item beyond 1000 rows, got %+v err=%v", pyHits, err)
	}
}

func TestDPAPICorruptPayloadErrors(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DPAPI encryption is Windows-only")
	}
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	it, err := st.SaveText("corrupt-check", SaveOptions{Tags: []string{SensitiveTag}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`UPDATE clip_items SET text_content = 'dpapi:!!!invalid-base64!!!' WHERE id = ?`, it.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetByID(it.ID); err == nil {
		t.Fatal("expected GetByID to return error on corrupted DPAPI payload")
	}
	if _, err := st.UpdateTags(it.ID, []string{"plain"}); err == nil {
		t.Fatal("expected UpdateTags to return error when decrypting corrupted DPAPI payload")
	}
}

func TestPinyinPolyphoneMemoAndInvalidRegexLines(t *testing.T) {
	// Polyphonic dense string with long mismatched ASCII query should return rapidly.
	text := strings.Repeat("长乐朝阳重行", 80)
	query := strings.Repeat("czx", 18) + "zzz"
	if matchPinyin(text, query) {
		t.Fatal("expected mismatched polyphonic query to return false")
	}

	invalid := InvalidCustomRuleLines("^valid-\\d+$\n# comment\n[unclosed\n\n(?invalid)")
	if len(invalid) != 2 || invalid[0] != 3 || invalid[1] != 5 {
		t.Fatalf("expected invalid line numbers [3, 5], got %v", invalid)
	}
}


func TestReorderPinnedPersistence(t *testing.T) {
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	a, _ := st.SaveText("pin-a", SaveOptions{})
	b, _ := st.SaveText("pin-b", SaveOptions{})
	c, _ := st.SaveText("pin-c", SaveOptions{})
	unpinned, _ := st.SaveText("unpinned", SaveOptions{})

	// Pinning a, b, c places the most recently pinned item first: [c, b, a].
	for _, id := range []int64{a.ID, b.ID, c.ID} {
		if pinned, err := st.TogglePin(id); err != nil || !pinned {
			t.Fatalf("pin %d: pinned=%v err=%v", id, pinned, err)
		}
	}

	// Reorder full pinned list to [a, c, b].
	if err := st.ReorderPinned([]int64{a.ID, c.ID, b.ID}); err != nil {
		t.Fatal(err)
	}
	got, err := st.ListSummaries("", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || got[0].ID != a.ID || got[1].ID != c.ID || got[2].ID != b.ID || got[3].ID != unpinned.ID {
		t.Fatalf("unexpected order after full reorder: got=[%d,%d,%d,%d], want=[%d,%d,%d,%d]",
			got[0].ID, got[1].ID, got[2].ID, got[3].ID, a.ID, c.ID, b.ID, unpinned.ID)
	}

	// Subset reorder (e.g. under a filter showing only [a, b]) swaps their slots while keeping c in place -> [b, c, a].
	if err := st.ReorderPinned([]int64{b.ID, a.ID}); err != nil {
		t.Fatal(err)
	}
	got, err = st.ListSummaries("", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].ID != b.ID || got[1].ID != c.ID || got[2].ID != a.ID {
		t.Fatalf("unexpected order after subset reorder: got=[%d,%d,%d], want=[%d,%d,%d]",
			got[0].ID, got[1].ID, got[2].ID, b.ID, c.ID, a.ID)
	}
}
