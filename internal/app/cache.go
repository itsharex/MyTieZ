package app

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	bitmapFetchTimeout = 5 * time.Second
	saveAsyncTimeout   = 10 * time.Second
	pasteTimeout       = 3 * time.Second
)

func runWithTimeout(timeout time.Duration, fn func() error) error {
	done := make(chan error, 1)
	go func() {
		done <- fn()
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
		return fmt.Errorf("timeout after %v", timeout)
	}
}

type bitmapCache struct {
	items   map[string]*ui.Bitmap
	failed  map[string]bool
	loading map[string]bool
}

func newBitmapCache() *bitmapCache {
	return &bitmapCache{
		items:   make(map[string]*ui.Bitmap),
		failed:  make(map[string]bool),
		loading: make(map[string]bool),
	}
}

func (bc *bitmapCache) clear() {
	bc.items = make(map[string]*ui.Bitmap)
}

func (bc *bitmapCache) len() int {
	if bc == nil {
		return 0
	}
	return len(bc.items)
}

func (bc *bitmapCache) get(win *mygo.Window, key string, maxCount int, fetch func() ([]byte, error)) *ui.Bitmap {
	return bc.getBitmap(win, key, maxCount, func() (*ui.Bitmap, error) {
		data, err := fetch()
		if err != nil || len(data) == 0 {
			return nil, err
		}
		return ui.DecodeBitmap(data)
	})
}

func (bc *bitmapCache) getBitmap(win *mygo.Window, key string, maxCount int, load func() (*ui.Bitmap, error)) *ui.Bitmap {
	if key == "" {
		return nil
	}
	if bmp, ok := bc.items[key]; ok {
		return bmp
	}
	if bc.failed[key] || bc.loading[key] || win == nil {
		return nil
	}
	bc.loading[key] = true
	go func() {
		var bmp *ui.Bitmap
		err := runWithTimeout(bitmapFetchTimeout, func() error {
			var loadErr error
			bmp, loadErr = load()
			return loadErr
		})
		win.Update(func() {
			delete(bc.loading, key)
			if err != nil || bmp == nil {
				bc.failed[key] = true
				return
			}
			if len(bc.items) >= maxCount {
				bc.items = make(map[string]*ui.Bitmap)
			}
			bc.items[key] = bmp
		})
	}()
	return nil
}

// decodeThumbBitmap decodes image bytes and downscales oversized images to fit
// within maxW x maxH so thumbnails do not retain full-resolution RGBA buffers
// or raw compressed bytes in memory.
func decodeThumbBitmap(data []byte, maxW, maxH int) (*ui.Bitmap, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("empty image")
	}
	if w > maxW || h > maxH {
		scale := min(float64(maxW)/float64(w), float64(maxH)/float64(h))
		dstW := max(1, int(float64(w)*scale+0.5))
		dstH := max(1, int(float64(h)*scale+0.5))
		img = downscaleBilinear(img, dstW, dstH)
	}
	return ui.NewBitmap(img), nil
}

func downscaleBilinear(src image.Image, dstW, dstH int) *image.RGBA {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	xRatio := float64(sw) / float64(dstW)
	yRatio := float64(sh) / float64(dstH)

	for y := 0; y < dstH; y++ {
		sy := (float64(y)+0.5)*yRatio - 0.5
		if sy < 0 {
			sy = 0
		}
		y0 := int(sy)
		y1 := min(y0+1, sh-1)
		wy := sy - float64(y0)
		for x := 0; x < dstW; x++ {
			sx := (float64(x)+0.5)*xRatio - 0.5
			if sx < 0 {
				sx = 0
			}
			x0 := int(sx)
			x1 := min(x0+1, sw-1)
			wx := sx - float64(x0)

			c00 := color.RGBAModel.Convert(src.At(sb.Min.X+x0, sb.Min.Y+y0)).(color.RGBA)
			c10 := color.RGBAModel.Convert(src.At(sb.Min.X+x1, sb.Min.Y+y0)).(color.RGBA)
			c01 := color.RGBAModel.Convert(src.At(sb.Min.X+x0, sb.Min.Y+y1)).(color.RGBA)
			c11 := color.RGBAModel.Convert(src.At(sb.Min.X+x1, sb.Min.Y+y1)).(color.RGBA)

			lerp := func(a, b, c, d uint8) uint8 {
				top := float64(a)*(1-wx) + float64(b)*wx
				bot := float64(c)*(1-wx) + float64(d)*wx
				return uint8(top*(1-wy) + bot*wy + 0.5)
			}
			dst.SetRGBA(x, y, color.RGBA{
				R: lerp(c00.R, c10.R, c01.R, c11.R),
				G: lerp(c00.G, c10.G, c01.G, c11.G),
				B: lerp(c00.B, c10.B, c01.B, c11.B),
				A: lerp(c00.A, c10.A, c01.A, c11.A),
			})
		}
	}
	return dst
}
