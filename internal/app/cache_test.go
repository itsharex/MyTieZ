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
