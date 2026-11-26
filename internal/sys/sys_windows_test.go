//go:build windows

package sys

import (
	"encoding/binary"
	"image"
	"image/color"
	"os"
	"os/exec"
	"slices"
	"testing"
	"time"
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

func TestStartForegroundWatcherAndZeroHWND(t *testing.T) {
	prev := lastExternalHWND.Swap(0x1234)
	defer lastExternalHWND.Store(prev)

	recordExternalForeground(0)
	if got := lastExternalHWND.Load(); got != 0x1234 {
		t.Fatalf("expected zero hwnd ignored, got %#x", got)
	}

	stop, ok := StartForegroundWatcher()
	if !ok {
		t.Fatalf("expected StartForegroundWatcher to succeed")
	}
	stop()
	stop() // idempotent
}

func TestForegroundWinEventProcFiltering(t *testing.T) {
	prev := lastExternalHWND.Swap(0x5678)
	defer lastExternalHWND.Store(prev)

	// Non-foreground event should be ignored.
	foregroundWinEventProc(0, eventSystemForeground+1, 0x9999, uintptr(objidWindow), uintptr(childidSelf), 0, 0)
	if got := lastExternalHWND.Load(); got != 0x5678 {
		t.Fatalf("expected non-foreground event ignored, got %#x", got)
	}

	// Non-window idObject should be ignored.
	foregroundWinEventProc(0, eventSystemForeground, 0x9999, 1, uintptr(childidSelf), 0, 0)
	if got := lastExternalHWND.Load(); got != 0x5678 {
		t.Fatalf("expected non-window idObject ignored, got %#x", got)
	}

	// Non-self idChild should be ignored.
	foregroundWinEventProc(0, eventSystemForeground, 0x9999, uintptr(objidWindow), 1, 0, 0)
	if got := lastExternalHWND.Load(); got != 0x5678 {
		t.Fatalf("expected non-self idChild ignored, got %#x", got)
	}
}

func TestElevatedRestartArgsAndWait(t *testing.T) {
	built := buildRestartAdminArgs(4321, []string{"--wait-pid=111", "--foo", "--wait-pid", "222", "bar"})
	wantBuilt := []string{"--foo", "bar", "--wait-pid=4321"}
	if !slices.Equal(built, wantBuilt) {
		t.Fatalf("buildRestartAdminArgs = %v, want %v", built, wantBuilt)
	}

	pid, cleaned := extractWaitPID([]string{"mytiez.exe", "--foo", "--wait-pid=4321", "bar"})
	if pid != 4321 {
		t.Fatalf("extractWaitPID pid = %d, want 4321", pid)
	}
	if !slices.Equal(cleaned, []string{"mytiez.exe", "--foo", "bar"}) {
		t.Fatalf("extractWaitPID cleaned = %v, want [mytiez.exe --foo bar]", cleaned)
	}

	pid2, cleaned2 := extractWaitPID([]string{"mytiez.exe", "--wait-pid", "9876", "--baz"})
	if pid2 != 9876 || !slices.Equal(cleaned2, []string{"mytiez.exe", "--baz"}) {
		t.Fatalf("extractWaitPID two-arg = (%d, %v), want (9876, [mytiez.exe --baz])", pid2, cleaned2)
	}

	// Verify waitForProcessExit returns promptly once a child process exits.
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start dummy child: %v", err)
	}
	start := time.Now()
	waitForProcessExit(cmd.Process.Pid, 5*time.Second)
	_ = cmd.Wait()
	if elapsed := time.Since(start); elapsed >= 3*time.Second {
		t.Fatalf("waitForProcessExit took too long (%v) for exited child", elapsed)
	}
}
