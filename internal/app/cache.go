package app

import (
	"fmt"
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

func (bc *bitmapCache) get(win *mygo.Window, key string, maxCount int, fetch func() ([]byte, error)) *ui.Bitmap {
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
		var data []byte
		err := runWithTimeout(bitmapFetchTimeout, func() error {
			var fetchErr error
			data, fetchErr = fetch()
			return fetchErr
		})
		win.Update(func() {
			delete(bc.loading, key)
			if err != nil || len(data) == 0 {
				bc.failed[key] = true
				return
			}
			bmp, decErr := ui.DecodeBitmap(data)
			if decErr != nil {
				bc.failed[key] = true
				return
			}
			if len(bc.items) > maxCount {
				bc.items = make(map[string]*ui.Bitmap)
			}
			bc.items[key] = bmp
		})
	}()
	return nil
}
