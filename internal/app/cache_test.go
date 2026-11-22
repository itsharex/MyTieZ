package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestDecodeThumbBitmapDownscales(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	for y := 0; y < 1080; y++ {
		for x := 0; x < 1920; x++ {
			src.SetRGBA(x, y, color.RGBA{R: 120, G: 180, B: 220, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}

	bmp, err := decodeThumbBitmap(buf.Bytes(), 720, 200)
	if err != nil {
		t.Fatal(err)
	}
	w, h := bmp.Size()
	if w > 720 || h > 200 || w <= 0 || h <= 0 {
		t.Fatalf("expected downscaled size within 720x200, got %dx%d", w, h)
	}
}

func TestBitmapCacheGenerationIncrementsOnClear(t *testing.T) {
	bc := newBitmapCache()
	g0 := bc.gen.Load()
	bc.items["k"] = nil
	bc.clear()
	if bc.gen.Load() != g0+1 || len(bc.items) != 0 {
		t.Fatalf("expected clear to increment gen (%d -> %d) and empty items", g0, bc.gen.Load())
	}
}

func TestDownscaleBilinearFastPaths(t *testing.T) {
	nrgba := image.NewNRGBA(image.Rect(0, 0, 40, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 40; x++ {
			nrgba.SetNRGBA(x, y, color.NRGBA{R: 200, G: 100, B: 40, A: 128})
		}
	}
	outNRGBA := downscaleBilinear(nrgba, 20, 10)
	if outNRGBA.Bounds().Dx() != 20 || outNRGBA.Bounds().Dy() != 10 {
		t.Fatalf("unexpected NRGBA downscale size: %v", outNRGBA.Bounds())
	}
	gotN := outNRGBA.RGBAAt(10, 5)
	// Alpha 128 premultiplies (200, 100, 40) -> ~(100, 50, 20, 128).
	if gotN.A != 128 || gotN.R < 99 || gotN.R > 101 || gotN.G < 49 || gotN.G > 51 || gotN.B < 19 || gotN.B > 21 {
		t.Fatalf("unexpected premultiplied pixel from NRGBA fast path: %+v", gotN)
	}

	rgba := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 40; x++ {
			rgba.SetRGBA(x, y, color.RGBA{R: 90, G: 140, B: 210, A: 255})
		}
	}
	outRGBA := downscaleBilinear(rgba, 16, 8)
	if outRGBA.Bounds().Dx() != 16 || outRGBA.Bounds().Dy() != 8 {
		t.Fatalf("unexpected RGBA downscale size: %v", outRGBA.Bounds())
	}
	gotR := outRGBA.RGBAAt(8, 4)
	if gotR != (color.RGBA{R: 90, G: 140, B: 210, A: 255}) {
		t.Fatalf("unexpected pixel from RGBA fast path: %+v", gotR)
	}
}

func TestDecodeThumbBitmapFile(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 1200, 600))
	path := filepath.Join(t.TempDir(), "sample.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, src); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	bmp, err := decodeThumbBitmapFile(path, 720, 200)
	if err != nil {
		t.Fatal(err)
	}
	w, h := bmp.Size()
	if w != 400 || h != 200 {
		t.Fatalf("expected 400x200 downscaled thumbnail, got %dx%d", w, h)
	}
}

func TestBitmapCacheByteBudgetEviction(t *testing.T) {
	bc := newBitmapCacheWithBudget(0, 0)
	// Each 100x100 bitmap is 40,000 bytes (RGBA).
	makeBmp := func() *ui.Bitmap {
		return ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 100, 100)))
	}
	bc.put("a", makeBmp())
	bc.put("b", makeBmp())
	bc.put("c", makeBmp())
	if got := bc.bytes(); got != 120000 {
		t.Fatalf("expected 120000 tracked bytes, got %d", got)
	}

	// Budget 80,000 keeps the 2 most recent entries (b, c) and evicts a.
	if !bc.pruneToBytes(80000) {
		t.Fatalf("expected pruneToBytes to evict oldest entry")
	}
	if bc.len() != 2 || bc.bytes() != 80000 {
		t.Fatalf("expected 2 entries / 80000 bytes, got %d / %d", bc.len(), bc.bytes())
	}
	if _, ok := bc.items["a"]; ok {
		t.Fatalf("expected oldest entry a evicted")
	}
	if _, ok := bc.items["c"]; !ok {
		t.Fatalf("expected newest entry c retained")
	}

	// Very tight budget still keeps at least the newest entry.
	bc.pruneToBytes(1)
	if bc.len() != 1 || bc.bytes() != 40000 {
		t.Fatalf("expected 1 newest entry retained under tight budget, got %d / %d", bc.len(), bc.bytes())
	}

	bc.clear()
	if bc.bytes() != 0 {
		t.Fatalf("expected 0 bytes after clear, got %d", bc.bytes())
	}
}
