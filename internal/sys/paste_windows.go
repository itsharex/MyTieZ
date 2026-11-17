//go:build windows

package sys

import (
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32            = windows.NewLazySystemDLL("user32.dll")
	procSendInput     = user32.NewProc("SendInput")
	procMapVirtualKey = user32.NewProc("MapVirtualKeyW")
)

const (
	inputKeyboard       = 1
	keyeventfExtended   = 0x0001
	keyeventfKeyUp      = 0x0002
	keyeventfUnicode    = 0x0004
	keyeventfScancode   = 0x0008
	mapvkVkToVsc        = 0
	vkReturn            = 0x0D
	vkShift             = 0x10
	vkControl           = 0x11
	vkMenu              = 0x12
	vkInsert            = 0x2D
	vkV                 = 0x56
	vkLWin              = 0x5B
	vkRWin              = 0x5C
)

type keyboardInput struct {
	wVk         uint16
	wScan       uint16
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

// inputStruct matches the Windows INPUT struct size on amd64/arm64 (40 bytes total:
// 4 bytes type + 4 bytes padding + 32 bytes union).
type inputStruct struct {
	inputType uint32
	_         uint32
	ki        keyboardInput
	_         [8]byte
}

func sendInputs(inputs ...inputStruct) error {
	if len(inputs) == 0 {
		return nil
	}
	n, _, err := procSendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		unsafe.Sizeof(inputs[0]),
	)
	if n == 0 {
		return err
	}
	return nil
}

func scanCode(vk uint16) uint16 {
	sc, _, _ := procMapVirtualKey.Call(uintptr(vk), mapvkVkToVsc)
	return uint16(sc)
}

func releaseModifiers() {
	_ = sendInputs(
		inputStruct{inputType: inputKeyboard, ki: keyboardInput{wVk: vkLWin, dwFlags: keyeventfKeyUp}},
		inputStruct{inputType: inputKeyboard, ki: keyboardInput{wVk: vkRWin, dwFlags: keyeventfKeyUp}},
		inputStruct{inputType: inputKeyboard, ki: keyboardInput{wVk: vkShift, dwFlags: keyeventfKeyUp}},
		inputStruct{inputType: inputKeyboard, ki: keyboardInput{wVk: vkControl, dwFlags: keyeventfKeyUp}},
		inputStruct{inputType: inputKeyboard, ki: keyboardInput{wVk: vkMenu, dwFlags: keyeventfKeyUp}},
	)
}

// SendPaste simulates Shift+Insert (the default paste scheme) into the focused window.
func SendPaste() error {
	return SendPasteWith("shift_insert", "", false)
}

// SendPasteWith simulates pasting into the currently focused window using the
// specified method ("shift_insert", "ctrl_v", or "game_mode").
func SendPasteWith(method, text string, isTextKind bool) error {
	releaseModifiers()
	time.Sleep(50 * time.Millisecond)

	effective := method
	if effective == "game_mode" && (!isTextKind || text == "") {
		effective = "ctrl_v"
	}

	switch effective {
	case "ctrl_v":
		ctrlScan := scanCode(vkControl)
		vScan := scanCode(vkV)
		if err := sendInputs(
			inputStruct{inputType: inputKeyboard, ki: keyboardInput{wScan: ctrlScan, dwFlags: keyeventfScancode}},
			inputStruct{inputType: inputKeyboard, ki: keyboardInput{wScan: vScan, dwFlags: keyeventfScancode}},
		); err != nil {
			return err
		}
		time.Sleep(50 * time.Millisecond)
		return sendInputs(
			inputStruct{inputType: inputKeyboard, ki: keyboardInput{wScan: vScan, dwFlags: keyeventfScancode | keyeventfKeyUp}},
			inputStruct{inputType: inputKeyboard, ki: keyboardInput{wScan: ctrlScan, dwFlags: keyeventfScancode | keyeventfKeyUp}},
		)

	case "game_mode":
		return sendGameModeText(text)

	default: // "shift_insert"
		shiftScan := scanCode(vkShift)
		insertScan := scanCode(vkInsert)
		if err := sendInputs(
			inputStruct{inputType: inputKeyboard, ki: keyboardInput{wVk: vkShift, wScan: shiftScan, dwFlags: keyeventfScancode}},
		); err != nil {
			return err
		}
		time.Sleep(10 * time.Millisecond)
		if err := sendInputs(
			inputStruct{inputType: inputKeyboard, ki: keyboardInput{wVk: vkInsert, wScan: insertScan, dwFlags: keyeventfExtended | keyeventfScancode}},
		); err != nil {
			return err
		}
		time.Sleep(10 * time.Millisecond)
		if err := sendInputs(
			inputStruct{inputType: inputKeyboard, ki: keyboardInput{wVk: vkInsert, wScan: insertScan, dwFlags: keyeventfKeyUp | keyeventfExtended | keyeventfScancode}},
		); err != nil {
			return err
		}
		time.Sleep(10 * time.Millisecond)
		return sendInputs(
			inputStruct{inputType: inputKeyboard, ki: keyboardInput{wVk: vkShift, wScan: shiftScan, dwFlags: keyeventfKeyUp | keyeventfScancode}},
		)
	}
}

func sendGameModeText(text string) error {
	time.Sleep(250 * time.Millisecond)
	targetHWND, _, _ := procGetForegroundWindow.Call()

	units := utf16.Encode([]rune(text))
	total := len(units)
	downDelay, upDelay, checkInterval := 10*time.Millisecond, 10*time.Millisecond, 20
	if total > 800 {
		downDelay, upDelay, checkInterval = 2*time.Millisecond, 2*time.Millisecond, 40
	} else if total > 200 {
		downDelay, upDelay, checkInterval = 4*time.Millisecond, 4*time.Millisecond, 30
	}

	enterScan := scanCode(vkReturn)
	for i, u := range units {
		if i%checkInterval == 0 && targetHWND != 0 {
			curHWND, _, _ := procGetForegroundWindow.Call()
			if curHWND != targetHWND {
				break
			}
		}
		if u == '\r' {
			continue
		}
		if u == '\n' {
			_ = sendInputs(inputStruct{inputType: inputKeyboard, ki: keyboardInput{wVk: vkReturn, wScan: enterScan, dwFlags: keyeventfScancode}})
			time.Sleep(downDelay)
			_ = sendInputs(inputStruct{inputType: inputKeyboard, ki: keyboardInput{wVk: vkReturn, wScan: enterScan, dwFlags: keyeventfScancode | keyeventfKeyUp}})
			time.Sleep(upDelay)
			continue
		}
		_ = sendInputs(inputStruct{inputType: inputKeyboard, ki: keyboardInput{wScan: u, dwFlags: keyeventfUnicode}})
		time.Sleep(downDelay)
		_ = sendInputs(inputStruct{inputType: inputKeyboard, ki: keyboardInput{wScan: u, dwFlags: keyeventfUnicode | keyeventfKeyUp}})
		time.Sleep(upDelay)
	}
	return nil
}
