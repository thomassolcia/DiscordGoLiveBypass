//go:build windows

package main

import (
	"os"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Mexe na bandeja de verdade, então só roda quando pedido: DGLB_BANDEJA=1 go test -run Bandeja.
func TestBandejaAoVivo(t *testing.T) {
	if os.Getenv("DGLB_BANDEJA") == "" {
		t.Skip("defina DGLB_BANDEJA=1 para pôr um ícone de teste na bandeja")
	}

	if size := unsafe.Sizeof(notifyIconData{}); size != 976 {
		t.Errorf("NOTIFYICONDATAW com %d bytes, o Windows espera 976", size)
	}
	if size := unsafe.Sizeof(wndClassEx{}); size != 80 {
		t.Errorf("WNDCLASSEXW com %d bytes, o Windows espera 80", size)
	}

	tray, err := startTray("DiscordGoLiveBypass (teste)", func() {}, func() []trayItem { return nil })
	if err != nil {
		t.Fatalf("startTray: %v", err)
	}
	defer tray.Close()

	class, _ := windows.UTF16PtrFromString("DiscordGoLiveBypassBandeja")
	found, _, _ := user32.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(class)), 0)
	if found == 0 {
		t.Error("a janela oculta do ícone não apareceu")
	}
	tray.SetTip("DiscordGoLiveBypass: teste trocando o texto")
	time.Sleep(time.Second)
}
