package store

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
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
	if !cfg.Deduplicate || cfg.MaxHistory != 500 || cfg.Hotkey != "Alt+V" {
		t.Fatalf("unexpected default settings: %+v", cfg)
	}

	cfg.Deduplicate = false
	cfg.CompactMode = true
	cfg.ColorMode = "dark"
	if err := st.SaveSettings(cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := st.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Deduplicate || !loaded.CompactMode || loaded.ColorMode != "dark" {
		t.Fatalf("unexpected loaded settings: %+v", loaded)
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
	if loaded.WindowWidth != 420 || loaded.WindowHeight != 600 {
		t.Fatalf("expected fallback window size 420x600, got %dx%d", loaded.WindowWidth, loaded.WindowHeight)
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
