//go:build windows

package sys

import (
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

func startLLHook(hookID, callback uintptr, onCleanup func()) (func(), bool) {
	tidReady := make(chan uint32, 1)
	hookOK := make(chan bool, 1)
	pumpDone := make(chan struct{})

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(pumpDone)

		var m winMsg
		// Force creation of the thread message queue before advertising tid.
		procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmNoRemove)
		tid := windows.GetCurrentThreadId()
		hhook, _, _ := procSetWindowsHookExW.Call(hookID, callback, 0, 0)
		tidReady <- tid
		hookOK <- (hhook != 0)
		if hhook == 0 {
			return
		}
		defer procUnhookWindowsHookEx.Call(hhook)

		for {
			r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(r) <= 0 {
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
	}()

	tid := <-tidReady
	ok := <-hookOK
	if !ok {
		if onCleanup != nil {
			onCleanup()
		}
		<-pumpDone
		return func() {}, false
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			if onCleanup != nil {
				onCleanup()
			}
			if tid != 0 {
				procPostThreadMessageW.Call(uintptr(tid), uintptr(wmQuit), 0, 0)
			}
			<-pumpDone
		})
	}, true
}
