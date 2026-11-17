//go:build windows

package sys

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32        = windows.NewLazySystemDLL("user32.dll")
	procSendInput = user32.NewProc("SendInput")
)

const (
	inputKeyboard  = 1
	keyeventfKeyUp = 0x0002
	vkControl      = 0x11
	vkV            = 0x56
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

// SendPaste simulates Ctrl+V to paste into the currently focused window.
func SendPaste() error {
	inputs := []inputStruct{
		{inputType: inputKeyboard, ki: keyboardInput{wVk: vkControl}},
		{inputType: inputKeyboard, ki: keyboardInput{wVk: vkV}},
		{inputType: inputKeyboard, ki: keyboardInput{wVk: vkV, dwFlags: keyeventfKeyUp}},
		{inputType: inputKeyboard, ki: keyboardInput{wVk: vkControl, dwFlags: keyeventfKeyUp}},
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
