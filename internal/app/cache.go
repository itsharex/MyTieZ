package app

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
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
	items      map[string]*ui.Bitmap
	order      []string
	failed     map[string]bool
	loadingMu  sync.Mutex
	loading    map[string]bool
	sem        chan struct{}
	maxBytes   int64
	totalBytes int64
	activeMu   sync.RWMutex
	activeKeys map[string]struct{}
	gen        atomic.Uint64
}

func newBitmapCache() *bitmapCache {
	return newBitmapCacheWithConcurrency(0)
}

func newBitmapCacheWithConcurrency(maxConc int) *bitmapCache {
	return newBitmapCacheWithBudget(maxConc, 0)
}

func newBitmapCacheWithBudget(maxConc int, maxBytes int64) *bitmapCache {
	var sem chan struct{}
	if maxConc > 0 {
		sem = make(chan struct{}, maxConc)
	}
	return &bitmapCache{
		items:    make(map[string]*ui.Bitmap),
		failed:   make(map[string]bool),
		loading:  make(map[string]bool),
		sem:      sem,
		maxBytes: maxBytes,
	}
}

func bitmapByteSize(bmp *ui.Bitmap) int64 {
	if bmp == nil {
		return 0
	}
	w, h := bmp.Size()
	if w <= 0 || h <= 0 {
		return 0
	}
	return int64(w) * int64(h) * 4
}

func (bc *bitmapCache) clear() {
	bc.gen.Add(1)
	bc.items = make(map[string]*ui.Bitmap)
	bc.order = nil
	bc.totalBytes = 0
	bc.setActiveKeys(nil)
	if len(bc.failed) > 128 {
		bc.failed = make(map[string]bool)
	}
}

func (bc *bitmapCache) setActiveKeys(keys map[string]struct{}) {
	if bc == nil {
		return
	}
	bc.activeMu.Lock()
	bc.activeKeys = keys
	bc.activeMu.Unlock()
}

func (bc *bitmapCache) markActive(key string) {
	if bc == nil || key == "" {
		return
	}
	bc.activeMu.Lock()
	if bc.activeKeys != nil {
		bc.activeKeys[key] = struct{}{}
	}
	bc.activeMu.Unlock()
}

func (bc *bitmapCache) isActive(key string) bool {
	if bc == nil {
		return false
	}
	bc.activeMu.RLock()
	defer bc.activeMu.RUnlock()
	if bc.activeKeys == nil {
		return true
	}
	_, ok := bc.activeKeys[key]
	return ok
}

func (bc *bitmapCache) len() int {
	if bc == nil {
		return 0
	}
	return len(bc.items)
}

func (bc *bitmapCache) recomputeBytes() int64 {
	var total int64
	for _, bmp := range bc.items {
		total += bitmapByteSize(bmp)
	}
	bc.totalBytes = total
	return total
}

func (bc *bitmapCache) bytes() int64 {
	if bc == nil {
		return 0
	}
	return bc.totalBytes
}

func (bc *bitmapCache) put(key string, bmp *ui.Bitmap) {
	if prev, ok := bc.items[key]; ok {
		bc.totalBytes -= bitmapByteSize(prev)
	}
	bc.items[key] = bmp
	bc.totalBytes += bitmapByteSize(bmp)
	if bc.totalBytes < 0 {
		bc.recomputeBytes()
	}
	bc.touch(key)
}

func (bc *bitmapCache) startLoading(key string) bool {
	bc.loadingMu.Lock()
	defer bc.loadingMu.Unlock()
	if bc.loading[key] {
		return false
	}
	bc.loading[key] = true
	return true
}

func (bc *bitmapCache) finishLoading(key string) {
	bc.loadingMu.Lock()
	delete(bc.loading, key)
	bc.loadingMu.Unlock()
}

func (bc *bitmapCache) activeStats(keys map[string]struct{}) (cachedCount int, cachedBytes int64, loadingCount int) {
	if bc == nil || len(keys) == 0 {
		return 0, 0, 0
	}
	for k := range keys {
		if bmp, ok := bc.items[k]; ok {
			cachedCount++
			cachedBytes += bitmapByteSize(bmp)
		}
	}
	bc.loadingMu.Lock()
	if len(bc.loading) <= len(keys) {
		for k := range bc.loading {
			if _, active := keys[k]; active {
				if _, cached := bc.items[k]; !cached {
					loadingCount++
				}
			}
		}
	} else {
		for k := range keys {
			if bc.loading[k] {
				if _, cached := bc.items[k]; !cached {
					loadingCount++
				}
			}
		}
	}
	bc.loadingMu.Unlock()
	return cachedCount, cachedBytes, loadingCount
}

func (bc *bitmapCache) hasOrPending(key string) bool {
	if bc == nil || key == "" {
		return false
	}
	if _, ok := bc.items[key]; ok {
		return true
	}
	if bc.failed[key] {
		return true
	}
	bc.loadingMu.Lock()
	pending := bc.loading[key]
	bc.loadingMu.Unlock()
	return pending
}

func (bc *bitmapCache) touch(key string) {
	n := len(bc.order)
	if n > 0 && bc.order[n-1] == key {
		return
	}
	for i, k := range bc.order {
		if k == key {
			copy(bc.order[i:], bc.order[i+1:])
			bc.order[n-1] = key
			return
		}
	}
	bc.order = append(bc.order, key)
}

func (bc *bitmapCache) touchIfPresent(key string) bool {
	if bc == nil || key == "" {
		return false
	}
	if _, ok := bc.items[key]; !ok {
		return false
	}
	bc.touch(key)
	return true
}

func (bc *bitmapCache) evict(key string) bool {
	if bc == nil || key == "" {
		return false
	}
	bmp, ok := bc.items[key]
	if !ok {
		return false
	}
	delete(bc.items, key)
	bc.totalBytes -= bitmapByteSize(bmp)
	if bc.totalBytes < 0 {
		bc.recomputeBytes()
	}
	for i, k := range bc.order {
		if k == key {
			copy(bc.order[i:], bc.order[i+1:])
			bc.order = bc.order[:len(bc.order)-1]
			break
		}
	}
	return true
}

// pruneTo keeps up to the keep most recently accessed entries and drops older ones.
func (bc *bitmapCache) pruneTo(keep int) bool {
	if bc == nil || len(bc.items) <= keep {
		return false
	}
	if keep <= 0 {
		bc.clear()
		return true
	}
	// If items were populated directly without touch (e.g. in tests), rebuild order.
	if len(bc.order) < len(bc.items) {
		for k := range bc.items {
			bc.touch(k)
		}
	}
	evict := len(bc.order) - keep
	if evict <= 0 {
		return false
	}
	for _, k := range bc.order[:evict] {
		if bmp, ok := bc.items[k]; ok {
			bc.totalBytes -= bitmapByteSize(bmp)
			delete(bc.items, k)
		}
	}
	if bc.totalBytes < 0 {
		bc.recomputeBytes()
	}
	copy(bc.order, bc.order[evict:])
	clear(bc.order[keep:])
	bc.order = bc.order[:keep]
	return true
}

// pruneToBytes evicts the oldest LRU entries until total estimated RGBA bytes
// fit within maxBytes, always preserving at least the most recently used entry.
func (bc *bitmapCache) pruneToBytes(maxBytes int64) bool {
	if bc == nil || maxBytes <= 0 || len(bc.items) <= 1 || bc.totalBytes <= maxBytes {
		return false
	}
	if len(bc.order) < len(bc.items) {
		for k := range bc.items {
			bc.touch(k)
		}
	}
	evicted := 0
	for evicted < len(bc.order)-1 && bc.totalBytes > maxBytes {
		k := bc.order[evicted]
		if bmp, ok := bc.items[k]; ok {
			bc.totalBytes -= bitmapByteSize(bmp)
			delete(bc.items, k)
		}
		evicted++
	}
	if evicted == 0 {
		return false
	}
	if bc.totalBytes < 0 {
		bc.recomputeBytes()
	}
	remain := len(bc.order) - evicted
	copy(bc.order, bc.order[evicted:])
	clear(bc.order[remain:])
	bc.order = bc.order[:remain]
	return true
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
		bc.touch(key)
		return bmp
	}
	if bc.failed[key] || win == nil || !bc.startLoading(key) {
		return nil
	}
	gen := bc.gen.Load()
	go func() {
		if bc.sem != nil {
			bc.sem <- struct{}{}
			defer func() { <-bc.sem }()
		}
		if bc.gen.Load() != gen || !bc.isActive(key) {
			bc.finishLoading(key)
			return
		}
		var bmp *ui.Bitmap
		err := runWithTimeout(bitmapFetchTimeout, func() error {
			var loadErr error
			bmp, loadErr = load()
			return loadErr
		})
		win.Update(func() {
			bc.finishLoading(key)
			if gen != bc.gen.Load() {
				return
			}
			if err != nil || bmp == nil {
				if len(bc.failed) >= 128 {
					bc.failed = make(map[string]bool)
				}
				bc.failed[key] = true
				return
			}
			if maxCount > 0 && len(bc.items) >= maxCount {
				bc.pruneTo(maxCount - 1)
			}
			bc.put(key, bmp)
			if bc.maxBytes > 0 {
				bc.pruneToBytes(bc.maxBytes)
			}
		})
	}()
	return nil
}

// preloadBitmap warms the cache asynchronously for keys near the viewport
// without touching existing entries' LRU order.
func (bc *bitmapCache) preloadBitmap(win *mygo.Window, key string, maxCount int, load func() (*ui.Bitmap, error)) {
	if bc == nil || key == "" {
		return
	}
	if _, ok := bc.items[key]; ok {
		return
	}
	_ = bc.getBitmap(win, key, maxCount, load)
}

// decodeThumbBitmap decodes image bytes and downscales oversized images to fit
// within maxW x maxH so thumbnails do not retain full-resolution RGBA buffers
// or raw compressed bytes in memory.
func decodeThumbBitmap(data []byte, maxW, maxH int) (*ui.Bitmap, error) {
	return decodeThumbBitmapReader(bytes.NewReader(data), maxW, maxH)
}

// decodeThumbBitmapFile streams and decodes an image file directly without
// allocating a full-file byte slice in heap memory.
func decodeThumbBitmapFile(path string, maxW, maxH int) (*ui.Bitmap, error) {
	img, _, err := decodeThumbImageFile(path, maxW, maxH)
	if err != nil {
		return nil, err
	}
	return ui.NewBitmap(img), nil
}

// loadOrCreateThumbBitmap loads a pre-scaled on-disk thumbnail when available,
// avoiding full-resolution PNG decoding on subsequent scrolls. On cache miss it
// streams and downscales imagePath, then persists the scaled thumbnail atomically.
func loadOrCreateThumbBitmap(imagePath, thumbPath string, maxW, maxH int) (*ui.Bitmap, error) {
	if thumbPath != "" {
		if img, _, err := decodeThumbImageFile(thumbPath, maxW, maxH); err == nil {
			return ui.NewBitmap(img), nil
		}
	}
	img, scaled, err := decodeThumbImageFile(imagePath, maxW, maxH)
	if err != nil {
		return nil, err
	}
	if thumbPath != "" && scaled {
		if _, statErr := os.Stat(imagePath); statErr == nil {
			saveThumbCacheFile(thumbPath, img)
		}
	}
	return ui.NewBitmap(img), nil
}

func saveThumbCacheFile(thumbPath string, img image.Image) {
	dir := filepath.Dir(thumbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, ".thumb-*.tmp")
	if err != nil {
		return
	}
	tmpPath := tmp.Name()
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(tmp, img); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return
	}
	if err := os.Rename(tmpPath, thumbPath); err != nil {
		_ = os.Remove(tmpPath)
	}
}

func decodeThumbImageFile(path string, maxW, maxH int) (image.Image, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	return decodeThumbImageReader(bufio.NewReaderSize(f, 32*1024), maxW, maxH)
}

func decodeThumbBitmapReader(r io.Reader, maxW, maxH int) (*ui.Bitmap, error) {
	img, _, err := decodeThumbImageReader(r, maxW, maxH)
	if err != nil {
		return nil, err
	}
	return ui.NewBitmap(img), nil
}

func decodeThumbImageReader(r io.Reader, maxW, maxH int) (image.Image, bool, error) {
	img, _, err := image.Decode(r)
	if err != nil {
		return nil, false, err
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, false, fmt.Errorf("empty image")
	}
	scaled := false
	if w > maxW || h > maxH {
		scale := min(float64(maxW)/float64(w), float64(maxH)/float64(h))
		dstW := max(1, int(float64(w)*scale+0.5))
		dstH := max(1, int(float64(h)*scale+0.5))
		img = downscaleBilinear(img, dstW, dstH)
		scaled = true
	}
	return img, scaled, nil
}

func downscaleBilinear(src image.Image, dstW, dstH int) *image.RGBA {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	xRatio := float64(sw) / float64(dstW)
	yRatio := float64(sh) / float64(dstH)

	switch m := src.(type) {
	case *image.NRGBA:
		downscalePixBilinear(m.Pix, m.Stride, sb.Min.X-m.Rect.Min.X, sb.Min.Y-m.Rect.Min.Y, sw, sh, dst, dstW, dstH, xRatio, yRatio, true)
		return dst
	case *image.RGBA:
		downscalePixBilinear(m.Pix, m.Stride, sb.Min.X-m.Rect.Min.X, sb.Min.Y-m.Rect.Min.Y, sw, sh, dst, dstW, dstH, xRatio, yRatio, false)
		return dst
	}

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

func downscalePixBilinear(pix []byte, stride, offX, offY, sw, sh int, dst *image.RGBA, dstW, dstH int, xRatio, yRatio float64, premultiply bool) {
	for y := 0; y < dstH; y++ {
		sy := (float64(y)+0.5)*yRatio - 0.5
		if sy < 0 {
			sy = 0
		}
		y0 := int(sy)
		y1 := min(y0+1, sh-1)
		wy := sy - float64(y0)
		wy0 := 1 - wy
		row0 := (offY+y0)*stride + offX*4
		row1 := (offY+y1)*stride + offX*4
		drow := y * dst.Stride
		for x := 0; x < dstW; x++ {
			sx := (float64(x)+0.5)*xRatio - 0.5
			if sx < 0 {
				sx = 0
			}
			x0 := int(sx)
			x1 := min(x0+1, sw-1)
			wx := sx - float64(x0)
			wx0 := 1 - wx

			i00 := row0 + x0*4
			i10 := row0 + x1*4
			i01 := row1 + x0*4
			i11 := row1 + x1*4

			r := (float64(pix[i00])*wx0+float64(pix[i10])*wx)*wy0 + (float64(pix[i01])*wx0+float64(pix[i11])*wx)*wy
			g := (float64(pix[i00+1])*wx0+float64(pix[i10+1])*wx)*wy0 + (float64(pix[i01+1])*wx0+float64(pix[i11+1])*wx)*wy
			b := (float64(pix[i00+2])*wx0+float64(pix[i10+2])*wx)*wy0 + (float64(pix[i01+2])*wx0+float64(pix[i11+2])*wx)*wy
			a := (float64(pix[i00+3])*wx0+float64(pix[i10+3])*wx)*wy0 + (float64(pix[i01+3])*wx0+float64(pix[i11+3])*wx)*wy

			di := drow + x*4
			if premultiply {
				af := a / 255.0
				dst.Pix[di] = uint8(r*af + 0.5)
				dst.Pix[di+1] = uint8(g*af + 0.5)
				dst.Pix[di+2] = uint8(b*af + 0.5)
				dst.Pix[di+3] = uint8(a + 0.5)
			} else {
				dst.Pix[di] = uint8(r + 0.5)
				dst.Pix[di+1] = uint8(g + 0.5)
				dst.Pix[di+2] = uint8(b + 0.5)
				dst.Pix[di+3] = uint8(a + 0.5)
			}
		}
	}
}
