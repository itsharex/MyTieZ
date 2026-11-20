//go:build windows

package sys

import (
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
