//go:build windows

package sys

import (
	"encoding/binary"
	"image"
	"image/color"
	"testing"
	"unsafe"
)

func TestInputStructLayout(t *testing.T) {
	if got := unsafe.Sizeof(inputStruct{}); got != 40 {
		t.Fatalf("expected 64-bit Windows INPUT struct size 40, got %d", got)
	}
}

func TestKeyRouterPredicate(t *testing.T) {
	if !shouldRouteKey(vkUp, false, false) || !shouldRouteKey('A', true, false) || !shouldRouteKey('1', true, false) {
		t.Fatal("expected navigation, Ctrl+A, and Ctrl+1 keys routed")
	}
	if shouldRouteKey('A', false, true) || shouldRouteKey('C', true, false) || shouldRouteKey(vkLShift, false, false) {
		t.Fatal("expected Alt combos, Ctrl+C, and raw modifiers not routed")
	}
	if !isModifierVK(vkLControl) || !isModifierVK(vkRWin) || isModifierVK('A') {
		t.Fatal("unexpected isModifierVK classification")
	}
	if !isPrintableVK('Z') || !isPrintableVK(vkSpace) || isPrintableVK(vkEscape) {
		t.Fatal("unexpected isPrintableVK classification")
	}
}

func TestExtraRoutedShortcut(t *testing.T) {
	t.Cleanup(func() { SetExtraRoutedShortcut(0, false, false, false) })

	SetExtraRoutedShortcut('P', false, false, true)
	if !matchesExtraRoutedShortcut('P', false, false, true) {
		t.Fatal("expected Alt+P to match extra routed shortcut")
	}
	if matchesExtraRoutedShortcut('P', true, false, false) {
		t.Fatal("expected Ctrl+P not to match Alt+P")
	}

	SetExtraRoutedShortcut(0, false, false, false)
	if matchesExtraRoutedShortcut('P', false, false, true) {
		t.Fatal("expected cleared extra routed shortcut not to match")
	}
}

func TestWriteDIBInto(t *testing.T) {
	checkHeader := func(t *testing.T, buf []byte, w, h int) {
		t.Helper()
		le := binary.LittleEndian
		if got := le.Uint32(buf[0:]); got != 40 {
			t.Fatalf("expected header size 40, got %d", got)
		}
		if got := int(le.Uint32(buf[4:])); got != w {
			t.Fatalf("expected width %d, got %d", w, got)
		}
		if got := int32(le.Uint32(buf[8:])); got != int32(-h) {
			t.Fatalf("expected top-down height %d, got %d", -h, got)
		}
		if got := le.Uint16(buf[14:]); got != 32 {
			t.Fatalf("expected 32 bpp, got %d", got)
		}
	}

	// NRGBA fast path
	nrgba := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	nrgba.SetNRGBA(0, 0, color.NRGBA{R: 10, G: 20, B: 30, A: 200})
	nrgba.SetNRGBA(1, 0, color.NRGBA{R: 40, G: 50, B: 60, A: 255})
	buf := make([]byte, 40+2*1*4)
	writeDIBInto(buf, nrgba)
	checkHeader(t, buf, 2, 1)
	if buf[40] != 30 || buf[41] != 20 || buf[42] != 10 || buf[43] != 200 {
		t.Fatalf("unexpected NRGBA pixel 0 BGRA: %v", buf[40:44])
	}
	if buf[44] != 60 || buf[45] != 50 || buf[46] != 40 || buf[47] != 255 {
		t.Fatalf("unexpected NRGBA pixel 1 BGRA: %v", buf[44:48])
	}

	// RGBA fast path (opaque and semi-transparent unpremultiply)
	rgba := image.NewRGBA(image.Rect(0, 0, 2, 1))
	rgba.SetRGBA(0, 0, color.RGBA{R: 50, G: 100, B: 150, A: 255})
	rgba.SetRGBA(1, 0, color.RGBA{R: 50, G: 0, B: 0, A: 128})
	writeDIBInto(buf, rgba)
	if buf[40] != 150 || buf[41] != 100 || buf[42] != 50 || buf[43] != 255 {
		t.Fatalf("unexpected opaque RGBA pixel 0 BGRA: %v", buf[40:44])
	}
	if buf[46] < 99 || buf[46] > 101 || buf[47] != 128 {
		t.Fatalf("unexpected unpremultiplied RGBA pixel 1 BGRA: %v", buf[44:48])
	}

	// Generic fallback (Gray)
	gray := image.NewGray(image.Rect(0, 0, 1, 1))
	gray.SetGray(0, 0, color.Gray{Y: 123})
	gbuf := make([]byte, 40+4)
	writeDIBInto(gbuf, gray)
	checkHeader(t, gbuf, 1, 1)
	if gbuf[40] != 123 || gbuf[41] != 123 || gbuf[42] != 123 || gbuf[43] != 255 {
		t.Fatalf("unexpected Gray pixel BGRA: %v", gbuf[40:44])
	}
}

func TestMatchesGlobalHotkey(t *testing.T) {
	target := globalHotkeyTarget{vk: vkV, win: true}
	if !matchesGlobalHotkey(target, vkV, true, false, false, false) {
		t.Fatalf("expected Win+V to match target")
	}
	if matchesGlobalHotkey(target, vkV, false, false, false, false) {
		t.Fatalf("expected bare V not to match Win+V")
	}
	if matchesGlobalHotkey(target, vkV, true, true, false, false) {
		t.Fatalf("expected Ctrl+Win+V not to match Win+V")
	}
	if matchesGlobalHotkey(target, 'C', true, false, false, false) {
		t.Fatalf("expected Win+C not to match Win+V")
	}
	if matchesGlobalHotkey(globalHotkeyTarget{}, vkV, true, false, false, false) {
		t.Fatalf("expected empty target not to match")
	}
}
