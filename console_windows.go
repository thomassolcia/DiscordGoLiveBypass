//go:build windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode     = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode     = kernel32.NewProc("SetConsoleMode")
	procSetConsoleOutputCP = kernel32.NewProc("SetConsoleOutputCP")
)

const (
	enableVirtualTerminal = 0x0004
	codePageUTF8          = 65001
)

// O console do Windows nasce sem interpretar ANSI e numa codepage antiga. Os dois
// precisam ser ligados na mao, e o que falhar apenas rebaixa a aparencia: quem nao
// consegue ANSI cai para linhas simples, quem nao consegue UTF-8 cai para ASCII.
func prepareConsole(f *os.File) (ansi bool, wide bool) {
	if !charDevice(f) {
		return false, false
	}

	handle := syscall.Handle(f.Fd())
	var mode uint32
	if ok, _, _ := procGetConsoleMode.Call(uintptr(handle), uintptr(unsafe.Pointer(&mode))); ok == 0 {
		return false, false
	}

	if mode&enableVirtualTerminal == 0 {
		if ok, _, _ := procSetConsoleMode.Call(uintptr(handle), uintptr(mode|enableVirtualTerminal)); ok == 0 {
			return false, false
		}
	}

	ok, _, _ := procSetConsoleOutputCP.Call(codePageUTF8)
	return true, ok != 0
}
