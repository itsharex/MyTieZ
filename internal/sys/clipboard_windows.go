//go:build windows

package sys

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// ErrClipboardUnsupported is returned on platforms where native clipboard image
// writing is not implemented and callers should fall back to MyGo's clipboard API.
var ErrClipboardUnsupported = errors.New("clipboard image write unsupported on this platform")

var (
	procOpenClipboard              = user32.NewProc("OpenClipboard")
	procCloseClipboard             = user32.NewProc("CloseClipboard")
	procEmptyClipboard             = user32.NewProc("EmptyClipboard")
	procSetClipboardData           = user32.NewProc("SetClipboardData")
	procRegisterClipboardFormatW   = user32.NewProc("RegisterClipboardFormatW")
	procGetClipboardSequenceNumber = user32.NewProc("GetClipboardSequenceNumber")

	procGlobalAlloc  = kernel32.NewProc("GlobalAlloc")
	procGlobalFree   = kernel32.NewProc("GlobalFree")
	procGlobalLock   = kernel32.NewProc("GlobalLock")
	procGlobalUnlock = kernel32.NewProc("GlobalUnlock")

	cfPNGOnce sync.Once
	cfPNGID   uintptr
)

const (
	cfDIB                 = 8
	gmemMoveable          = 0x0002
	maxClipboardImageFile = 64 << 20
	maxClipboardDIBBytes  = 256 << 20
)

func registeredPNGFormat() uintptr {
	cfPNGOnce.Do(func() {
		name, err := syscall.UTF16PtrFromString("PNG")
		if err != nil {
			return
		}
		cfPNGID, _, _ = procRegisterClipboardFormatW.Call(uintptr(unsafe.Pointer(name)))
	})
	return cfPNGID
}

// ClipboardSequenceNumber returns the Windows clipboard sequence number, which
// increments on every clipboard change.
func ClipboardSequenceNumber() uint32 {
	seq, _, _ := procGetClipboardSequenceNumber.Call()
	return uint32(seq)
}

func allocHGlobal(size int64) (uintptr, error) {
	if size <= 0 {
		return 0, errors.New("empty clipboard allocation")
	}
	h, _, err := procGlobalAlloc.Call(gmemMoveable, uintptr(size))
	if h == 0 {
		return 0, fmt.Errorf("GlobalAlloc(%d): %w", size, err)
	}
	return h, nil
}

func lockHGlobalSlice(h uintptr, size int64) ([]byte, error) {
	p, _, err := procGlobalLock.Call(h)
	if p == 0 {
		return nil, fmt.Errorf("GlobalLock: %w", err)
	}
	var ptr unsafe.Pointer
	*(*uintptr)(unsafe.Pointer(&ptr)) = p
	return unsafe.Slice((*byte)(ptr), int(size)), nil
}

// WriteClipboardImageFile writes an image file directly to the Windows clipboard
// as both registered "PNG" and standard CF_DIB using OS-owned HGLOBAL buffers.
// Unlike delayed OLE rendering, it avoids keeping the raw PNG in Go memory and
// writes DIB pixels directly into Win32 memory without an intermediate Go []byte.
func WriteClipboardImageFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}
	fileSize := info.Size()
	if fileSize <= 0 || fileSize > maxClipboardImageFile {
		return fmt.Errorf("invalid clipboard image size: %d", fileSize)
	}

	hPNG, err := allocHGlobal(fileSize)
	if err != nil {
		return err
	}
	defer func() {
		if hPNG != 0 {
			_, _, _ = procGlobalFree.Call(hPNG)
		}
	}()

	pngDst, err := lockHGlobalSlice(hPNG, fileSize)
	if err != nil {
		return err
	}
	_, readErr := io.ReadFull(f, pngDst)
	_, _, _ = procGlobalUnlock.Call(hPNG)
	if readErr != nil {
		return readErr
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	img, _, err := image.Decode(bufio.NewReaderSize(f, 32*1024))
	if err != nil {
		return err
	}

	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return errors.New("empty decoded image")
	}
	dibSize := int64(40) + int64(w)*int64(h)*4
	if dibSize <= 40 || dibSize > maxClipboardDIBBytes {
		return fmt.Errorf("decoded DIB exceeds limit: %dx%d", w, h)
	}

	hDIB, err := allocHGlobal(dibSize)
	if err != nil {
		return err
	}
	defer func() {
		if hDIB != 0 {
			_, _, _ = procGlobalFree.Call(hDIB)
		}
	}()

	dibDst, err := lockHGlobalSlice(hDIB, dibSize)
	if err != nil {
		return err
	}
	writeDIBInto(dibDst, img)
	_, _, _ = procGlobalUnlock.Call(hDIB)
	img = nil

	opened := false
	for range 10 {
		if r, _, _ := procOpenClipboard.Call(0); r != 0 {
			opened = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !opened {
		return errors.New("OpenClipboard failed: clipboard busy")
	}
	defer procCloseClipboard.Call()

	if r, _, err := procEmptyClipboard.Call(); r == 0 {
		return fmt.Errorf("EmptyClipboard: %w", err)
	}

	if cfPNG := registeredPNGFormat(); cfPNG != 0 {
		if r, _, _ := procSetClipboardData.Call(cfPNG, hPNG); r != 0 {
			hPNG = 0
		}
	}
	if r, _, err := procSetClipboardData.Call(cfDIB, hDIB); r == 0 {
		return fmt.Errorf("SetClipboardData(CF_DIB): %w", err)
	}
	hDIB = 0
	return nil
}

// writeDIBInto encodes img as a top-down 32-bit BITMAPINFOHEADER DIB directly into out.
func writeDIBInto(out []byte, img image.Image) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	le := binary.LittleEndian
	clear(out[:40])
	le.PutUint32(out[0:], 40)
	le.PutUint32(out[4:], uint32(w))
	le.PutUint32(out[8:], uint32(int32(-h)))
	le.PutUint16(out[12:], 1)
	le.PutUint16(out[14:], 32)
	le.PutUint32(out[20:], uint32(4*w*h))

	di := 40
	switch m := img.(type) {
	case *image.NRGBA:
		offX := b.Min.X - m.Rect.Min.X
		offY := b.Min.Y - m.Rect.Min.Y
		for y := 0; y < h; y++ {
			si := (offY+y)*m.Stride + offX*4
			for x := 0; x < w; x++ {
				out[di] = m.Pix[si+2]
				out[di+1] = m.Pix[si+1]
				out[di+2] = m.Pix[si]
				out[di+3] = m.Pix[si+3]
				si += 4
				di += 4
			}
		}
		return
	case *image.RGBA:
		offX := b.Min.X - m.Rect.Min.X
		offY := b.Min.Y - m.Rect.Min.Y
		for y := 0; y < h; y++ {
			si := (offY+y)*m.Stride + offX*4
			for x := 0; x < w; x++ {
				r, g, bl, a := m.Pix[si], m.Pix[si+1], m.Pix[si+2], m.Pix[si+3]
				if a == 255 || a == 0 {
					out[di], out[di+1], out[di+2], out[di+3] = bl, g, r, a
				} else {
					a32 := uint32(a)
					out[di] = uint8(min(255, (uint32(bl)*255+a32/2)/a32))
					out[di+1] = uint8(min(255, (uint32(g)*255+a32/2)/a32))
					out[di+2] = uint8(min(255, (uint32(r)*255+a32/2)/a32))
					out[di+3] = a
				}
				si += 4
				di += 4
			}
		}
		return
	}

	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			out[di], out[di+1], out[di+2], out[di+3] = c.B, c.G, c.R, c.A
			di += 4
		}
	}
}
