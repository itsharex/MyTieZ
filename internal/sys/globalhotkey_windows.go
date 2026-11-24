//go:build windows

package sys

import (
	"sync"
	"sync/atomic"
	"syscall"
)

type globalHotkeyTarget struct {
	vk    uint32
	win   bool
	ctrl  bool
	shift bool
	alt   bool
}

type globalHotkeyState struct {
	mu         sync.Mutex
	target     globalHotkeyTarget
	onTrigger  func()
	events     chan func()
	stopCh     chan struct{}
	stopHook   func()
	suppressed [256]bool
}

var (
	globalHotkeyOnce sync.Once
	globalHotkeyCB   uintptr
	activeGlobalHK   atomic.Pointer[globalHotkeyState]
)

// vkMaskKey 是用于通知 Windows Shell “Win 组合键已使用”的未分配虚拟键码，
// 防止拦截 Win+V 后松开 Win 键弹出开始菜单。
const vkMaskKey uint16 = 0xFF

func matchesGlobalHotkey(target globalHotkeyTarget, vk uint32, win, ctrl, shift, alt bool) bool {
	return target.vk != 0 &&
		target.vk == vk &&
		target.win == win &&
		target.ctrl == ctrl &&
		target.shift == shift &&
		target.alt == alt
}

func sendWinMaskKey() {
	_ = sendInputs(
		inputStruct{
			inputType: inputKeyboard,
			ki:        keyboardInput{wVk: vkMaskKey},
		},
		inputStruct{
			inputType: inputKeyboard,
			ki:        keyboardInput{wVk: vkMaskKey, dwFlags: keyeventfKeyUp},
		},
	)
}

func ensureGlobalHotkeyHook() *globalHotkeyState {
	if st := activeGlobalHK.Load(); st != nil {
		return st
	}
	globalHotkeyOnce.Do(func() {
		globalHotkeyCB = syscall.NewCallback(lowLevelGlobalHotkeyProc)
	})

	st := &globalHotkeyState{
		events: make(chan func(), 16),
		stopCh: make(chan struct{}),
	}
	if !activeGlobalHK.CompareAndSwap(nil, st) {
		return activeGlobalHK.Load()
	}

	go func() {
		for {
			select {
			case fn := <-st.events:
				if fn != nil {
					fn()
				}
			case <-st.stopCh:
				return
			}
		}
	}()

	stop, ok := startLLHook(whKeyboardLL, globalHotkeyCB, func() {
		activeGlobalHK.CompareAndSwap(st, nil)
		close(st.stopCh)
	})
	if !ok {
		activeGlobalHK.CompareAndSwap(st, nil)
		close(st.stopCh)
		return nil
	}
	st.mu.Lock()
	st.stopHook = stop
	st.mu.Unlock()
	return st
}

// StartGlobalHotkeyHook 安装全局低级键盘钩子，接管被系统占用的 Win+* 热键（如 Win+V）。
// 返回停止函数；若不需要钩子请调用 StopGlobalHotkeyHook。
func StartGlobalHotkeyHook(vk uint32, win, ctrl, shift, alt bool, onTrigger func()) (func(), bool) {
	if vk == 0 || vk >= 256 || onTrigger == nil {
		return func() {}, false
	}
	st := ensureGlobalHotkeyHook()
	if st == nil {
		return func() {}, false
	}
	st.mu.Lock()
	st.target = globalHotkeyTarget{vk: vk, win: win, ctrl: ctrl, shift: shift, alt: alt}
	st.onTrigger = onTrigger
	st.suppressed = [256]bool{}
	st.mu.Unlock()
	return StopGlobalHotkeyHook, true
}

// StopGlobalHotkeyHook 卸载全局热键低级键盘钩子。
func StopGlobalHotkeyHook() {
	st := activeGlobalHK.Load()
	if st == nil {
		return
	}
	st.mu.Lock()
	st.target = globalHotkeyTarget{}
	st.onTrigger = nil
	st.suppressed = [256]bool{}
	stopFn := st.stopHook
	st.mu.Unlock()
	if stopFn != nil {
		stopFn()
	}
}

func lowLevelGlobalHotkeyProc(nCode int32, wp, lp uintptr) uintptr {
	if nCode >= 0 && lp != 0 {
		if st := activeGlobalHK.Load(); st != nil {
			info := (*kbdllHookStruct)(nativePtr(lp))
			if (info.flags & llkhfInjected) != 0 {
				r, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wp, lp)
				return r
			}
			vk := info.vkCode
			if vk < 256 && !isModifierVK(vk) {
				switch wp {
				case wmKeyUp, wmSysKeyUp:
					st.mu.Lock()
					wasSuppressed := st.suppressed[vk]
					if wasSuppressed {
						st.suppressed[vk] = false
					}
					st.mu.Unlock()
					if wasSuppressed {
						return 1
					}
				case wmKeyDown, wmSysKeyDown:
					winDown := keyDownAsync(vkLWin) || keyDownAsync(vkRWin)
					ctrl := keyDownAsync(vkControl) || keyDownAsync(vkLControl) || keyDownAsync(vkRControl)
					shift := keyDownAsync(vkShift) || keyDownAsync(vkLShift) || keyDownAsync(vkRShift)
					alt := keyDownAsync(vkMenu) || keyDownAsync(vkLMenu) || keyDownAsync(vkRMenu)

					st.mu.Lock()
					target := st.target
					onTrigger := st.onTrigger
					alreadyPressed := st.suppressed[vk]
					st.mu.Unlock()

					if onTrigger != nil && matchesGlobalHotkey(target, vk, winDown, ctrl, shift, alt) {
						st.mu.Lock()
						st.suppressed[vk] = true
						st.mu.Unlock()
						if !alreadyPressed {
							select {
							case st.events <- func() {
								if winDown {
									sendWinMaskKey()
								}
								onTrigger()
							}:
							default:
							}
						}
						return 1
					}
				}
			}
		}
	}
	r, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wp, lp)
	return r
}
