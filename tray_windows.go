//go:build windows

package main

import (
	"errors"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Ícone na bandeja escrito direto na API do Windows. O Wails estável não tem bandeja, e as
// bibliotecas prontas querem ser donas da thread principal, que é do Wails. Aqui o ícone
// vive numa thread própria, com uma janela invisível e o laço de mensagens dela.

var (
	user32                     = windows.NewLazySystemDLL("user32.dll")
	procRegisterClassExW       = user32.NewProc("RegisterClassExW")
	procCreateWindowExW        = user32.NewProc("CreateWindowExW")
	procDefWindowProcW         = user32.NewProc("DefWindowProcW")
	procDestroyWindow          = user32.NewProc("DestroyWindow")
	procGetMessageW            = user32.NewProc("GetMessageW")
	procTranslateMessage       = user32.NewProc("TranslateMessage")
	procDispatchMessageW       = user32.NewProc("DispatchMessageW")
	procPostMessageW           = user32.NewProc("PostMessageW")
	procPostQuitMessage        = user32.NewProc("PostQuitMessage")
	procCreatePopupMenu        = user32.NewProc("CreatePopupMenu")
	procAppendMenuW            = user32.NewProc("AppendMenuW")
	procSetMenuDefaultItem     = user32.NewProc("SetMenuDefaultItem")
	procTrackPopupMenu         = user32.NewProc("TrackPopupMenu")
	procDestroyMenu            = user32.NewProc("DestroyMenu")
	procSetForegroundWindow    = user32.NewProc("SetForegroundWindow")
	procGetCursorPos           = user32.NewProc("GetCursorPos")
	procRegisterWindowMessageW = user32.NewProc("RegisterWindowMessageW")
	procLoadImageW             = user32.NewProc("LoadImageW")
	procGetSystemMetrics       = user32.NewProc("GetSystemMetrics")
	procShellNotifyIconW       = windows.NewLazySystemDLL("shell32.dll").NewProc("Shell_NotifyIconW")
)

const (
	wmDestroy       = 0x0002
	wmClose         = 0x0010
	wmNull          = 0x0000
	wmTray          = 0x8000 + 1
	wmLButtonUp     = 0x0202
	wmLButtonDblClk = 0x0203
	wmRButtonUp     = 0x0205

	nimAdd     = 0
	nimModify  = 1
	nimDelete  = 2
	nifMessage = 0x1
	nifIcon    = 0x2
	nifTip     = 0x4
	nifInfo    = 0x10
	niifUser   = 0x4

	mfString       = 0x0
	mfGrayed       = 0x1
	mfSeparator    = 0x800
	tpmRightButton = 0x0002
	tpmBottomAlign = 0x0020
	tpmNoNotify    = 0x0080
	tpmReturnCmd   = 0x0100

	imageIcon      = 1
	lrShared       = 0x8000
	smCxSmIcon     = 49
	smCySmIcon     = 50
	idiApplication = 32512
	appIconID      = 3
	firstMenuID    = 1000
)

type notifyIconData struct {
	Size            uint32
	Wnd             uintptr
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            uintptr
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GUIDItem        windows.GUID
	BalloonIcon     uintptr
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type msg struct {
	Wnd     uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	X, Y    int32
	Private uint32
}

// trayItem é uma linha do menu. Sem Action, ela vira separador.
type trayItem struct {
	Label    string
	Action   func()
	Disabled bool
	Default  bool
}

type tray struct {
	mu             sync.Mutex
	wnd            uintptr
	icon           uintptr
	tip            string
	taskbarCreated uintptr
	onOpen         func()
	menu           func() []trayItem
}

// A janela invisível chama de volta uma função só, então o ícone ativo fica aqui.
var activeTray *tray

var trayWndProc = windows.NewCallback(func(wnd, message, wParam, lParam uintptr) uintptr {
	t := activeTray
	switch {
	case t == nil:
	case message == wmTray:
		switch lParam & 0xffff {
		case wmLButtonUp, wmLButtonDblClk:
			go t.onOpen()
		case wmRButtonUp:
			t.showMenu()
		}
		return 0
	case message == t.taskbarCreated && message != 0:
		// O Explorer reiniciou e levou a bandeja junto: o ícone precisa ser posto de novo.
		t.notify(nimAdd)
		return 0
	case message == wmClose:
		t.notify(nimDelete)
		procDestroyWindow.Call(wnd)
		return 0
	case message == wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	result, _, _ := procDefWindowProcW.Call(wnd, message, wParam, lParam)
	return result
})

func loadAppIcon() uintptr {
	var module windows.Handle
	windows.GetModuleHandleEx(0, nil, &module)
	cx, _, _ := procGetSystemMetrics.Call(smCxSmIcon)
	cy, _, _ := procGetSystemMetrics.Call(smCySmIcon)
	if icon, _, _ := procLoadImageW.Call(uintptr(module), appIconID, imageIcon, cx, cy, 0); icon != 0 {
		return icon
	}
	// Build feito sem o winres não tem o ícone embutido: fica o genérico do Windows.
	icon, _, _ := procLoadImageW.Call(0, idiApplication, imageIcon, 0, 0, lrShared)
	return icon
}

func startTray(tip string, onOpen func(), menu func() []trayItem) (*tray, error) {
	t := &tray{tip: tip, onOpen: onOpen, menu: menu}
	ready := make(chan error, 1)

	go func() {
		// A janela pertence à thread que a criou, e o laço de mensagens precisa rodar nela.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		if err := t.create(); err != nil {
			ready <- err
			return
		}
		ready <- nil

		var m msg
		for {
			result, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(result) <= 0 {
				return
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
	}()

	// Sem ícone, quem chama não pode esconder a janela ao minimizar: ela sumiria sem ter
	// por onde voltar. Por isso a falha devolve nil, e não um ícone pela metade.
	if err := <-ready; err != nil {
		return nil, err
	}
	return t, nil
}

func (t *tray) create() error {
	var module windows.Handle
	windows.GetModuleHandleEx(0, nil, &module)

	className, _ := windows.UTF16PtrFromString("DiscordGoLiveBypassBandeja")
	class := wndClassEx{WndProc: trayWndProc, Instance: uintptr(module), ClassName: className}
	class.Size = uint32(unsafe.Sizeof(class))
	if atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class))); atom == 0 && !errors.Is(err, windows.ERROR_CLASS_ALREADY_EXISTS) {
		return err
	}

	// Janela comum e nunca mostrada, e não uma message-only: só as comuns recebem o aviso
	// de que o Explorer reiniciou.
	wnd, _, err := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(className)),
		0, 0, 0, 0, 0, 0, 0, uintptr(module), 0)
	if wnd == 0 {
		return err
	}

	name, _ := windows.UTF16PtrFromString("TaskbarCreated")
	t.taskbarCreated, _, _ = procRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(name)))
	t.wnd = wnd
	t.icon = loadAppIcon()
	activeTray = t

	if !t.notify(nimAdd) {
		return errors.New("o Windows recusou o ícone na bandeja")
	}
	return nil
}

func (t *tray) notify(action uintptr) bool {
	t.mu.Lock()
	data := notifyIconData{Wnd: t.wnd, ID: 1, Flags: nifMessage | nifIcon | nifTip, CallbackMessage: wmTray, Icon: t.icon}
	tip, _ := windows.UTF16FromString(t.tip)
	copy(data.Tip[:len(data.Tip)-1], tip)
	t.mu.Unlock()

	data.Size = uint32(unsafe.Sizeof(data))
	ok, _, _ := procShellNotifyIconW.Call(action, uintptr(unsafe.Pointer(&data)))
	return ok != 0
}

// SetTip troca o texto que aparece ao passar o mouse no ícone.
func (t *tray) SetTip(text string) {
	t.mu.Lock()
	changed := t.tip != text
	t.tip = text
	t.mu.Unlock()
	if changed {
		t.notify(nimModify)
	}
}

// Balloon mostra uma notificação saindo do ícone, com o próprio ícone do programa. No Windows
// 10 e 11 ela vira um aviso comum, que também fica na central de notificações.
func (t *tray) Balloon(title, text string) bool {
	t.mu.Lock()
	data := notifyIconData{Wnd: t.wnd, ID: 1, Flags: nifInfo, InfoFlags: niifUser, BalloonIcon: t.icon}
	t.mu.Unlock()
	titleUTF16, _ := windows.UTF16FromString(title)
	textUTF16, _ := windows.UTF16FromString(text)
	copy(data.InfoTitle[:len(data.InfoTitle)-1], titleUTF16)
	copy(data.Info[:len(data.Info)-1], textUTF16)
	data.Size = uint32(unsafe.Sizeof(data))
	ok, _, _ := procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&data)))
	return ok != 0
}

func (t *tray) showMenu() {
	items := t.menu()
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)

	for i, item := range items {
		if item.Action == nil {
			procAppendMenuW.Call(menu, mfSeparator, 0, 0)
			continue
		}
		flags := uintptr(mfString)
		if item.Disabled {
			flags |= mfGrayed
		}
		label, _ := windows.UTF16PtrFromString(item.Label)
		procAppendMenuW.Call(menu, flags, uintptr(firstMenuID+i), uintptr(unsafe.Pointer(label)))
		if item.Default {
			procSetMenuDefaultItem.Call(menu, uintptr(firstMenuID+i), 0)
		}
	}

	var point struct{ X, Y int32 }
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&point)))
	// Sem trazer a janela para a frente antes, o menu não fecha quando a pessoa clica fora.
	procSetForegroundWindow.Call(t.wnd)
	chosen, _, _ := procTrackPopupMenu.Call(menu, tpmRightButton|tpmBottomAlign|tpmNoNotify|tpmReturnCmd,
		uintptr(point.X), uintptr(point.Y), 0, t.wnd, 0)
	procPostMessageW.Call(t.wnd, wmNull, 0, 0)

	if index := int(chosen) - firstMenuID; chosen != 0 && index >= 0 && index < len(items) && items[index].Action != nil {
		go items[index].Action()
	}
}

func (t *tray) Close() {
	if t != nil && t.wnd != 0 {
		procPostMessageW.Call(t.wnd, wmClose, 0, 0)
	}
}
