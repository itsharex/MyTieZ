package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
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
