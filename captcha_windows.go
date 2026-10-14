//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"github.com/wailsapp/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"
)

// A página de CAPTCHA do Proton recusa ser mostrada dentro de outra página
// (frame-ancestors), mas foi feita para rodar dentro de um WebView2: quando a pessoa
// resolve, ela entrega o token com chrome.webview.postMessage. Por isso o desafio abre
// numa janela própria, com um WebView2 só dela, num processo separado do programa:
// a biblioteca do WebView2 encerra o processo inteiro em qualquer erro, e assim só a
// janelinha cai, nunca a janela principal.

const (
	wmSize             = 0x0005
	wsOverlappedWindow = 0x00CF0000
	wsVisible          = 0x10000000
	smCxScreen         = 0
	smCyScreen         = 1
)

var procGetDpiForSystem = user32.NewProc("GetDpiForSystem")

// A página manda objetos, e a biblioteca só aceita texto: um objeto derrubaria o
// processo. Antes de qualquer script da página, o envio passa a converter tudo em JSON.
const captchaBridge = `(() => {
  const w = window.chrome && window.chrome.webview;
  if (!w) return;
  const post = w.postMessage.bind(w);
  w.postMessage = (m) => post(typeof m === 'string' ? m : JSON.stringify(m));
})();`

type captchaWindow struct {
	wnd      uintptr
	chromium *edge.Chromium
	result   chan captchaResult
}

type captchaResult struct {
	token string
	err   error
}

var activeCaptcha *captchaWindow

var captchaWndProc = windows.NewCallback(func(wnd, message, wParam, lParam uintptr) uintptr {
	c := activeCaptcha
	switch {
	case c == nil:
	case message == wmSize && c.chromium != nil:
		c.chromium.Resize()
		return 0
	case message == wmClose:
		select {
		case c.result <- captchaResult{err: errors.New("a verificação foi fechada antes de terminar")}:
		default:
		}
		procDestroyWindow.Call(wnd)
		return 0
	case message == wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	result, _, _ := procDefWindowProcW.Call(wnd, message, wParam, lParam)
	return result
})

// validCaptchaToken confere que a resposta é deste desafio: o Proton só aceita de volta
// "<desafio>:<resposta>".
func validCaptchaToken(token, challenge string) bool {
	prefix := challenge + ":"
	return challenge != "" && len(token) <= 16384 && strings.HasPrefix(token, prefix) && len(token) > len(prefix)
}

// runCaptchaWindow é o modo -captcha: mostra a página, espera a resposta e devolve o token.
func runCaptchaWindow(pageURL, challenge string) (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	c := &captchaWindow{result: make(chan captchaResult, 1)}
	activeCaptcha = c

	var module windows.Handle
	windows.GetModuleHandleEx(0, nil, &module)
	className, _ := windows.UTF16PtrFromString("DiscordGoLiveBypassVerificacao")
	title, _ := windows.UTF16PtrFromString("Verificação do Proton")
	class := wndClassEx{WndProc: captchaWndProc, Instance: uintptr(module), ClassName: className, Icon: loadAppIcon(), IconSm: loadAppIcon()}
	class.Size = uint32(unsafe.Sizeof(class))
	if atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class))); atom == 0 && !errors.Is(err, windows.ERROR_CLASS_ALREADY_EXISTS) {
		return "", err
	}

	dpi, _, _ := procGetDpiForSystem.Call()
	if dpi == 0 {
		dpi = 96
	}
	width, height := 460*dpi/96, 640*dpi/96
	screenW, _, _ := procGetSystemMetrics.Call(smCxScreen)
	screenH, _, _ := procGetSystemMetrics.Call(smCyScreen)
	x, y := (screenW-width)/2, (screenH-height)/2

	wnd, _, err := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)),
		wsOverlappedWindow|wsVisible, x, y, width, height, 0, 0, uintptr(module), 0)
	if wnd == 0 {
		return "", err
	}
	c.wnd = wnd
	procSetForegroundWindow.Call(wnd)

	chromium := edge.NewChromium()
	if dir, err := dataDir(); err == nil {
		chromium.DataPath = filepath.Join(dir, "verificacao")
	}
	chromium.MessageCallback = func(message string, _ *edge.ICoreWebView2, _ *edge.ICoreWebView2WebMessageReceivedEventArgs) {
		var data struct {
			Type  string `json:"type"`
			Token string `json:"token"`
		}
		if json.Unmarshal([]byte(message), &data) != nil || data.Type != "pm_captcha" || !validCaptchaToken(data.Token, challenge) {
			return
		}
		select {
		case c.result <- captchaResult{token: data.Token}:
		default:
		}
		procPostMessageW.Call(c.wnd, wmClose, 0, 0)
	}
	if !chromium.Embed(wnd) {
		return "", errors.New("o navegador embutido do Windows não abriu")
	}
	c.chromium = chromium
	chromium.Init(captchaBridge)
	chromium.Resize()
	chromium.Navigate(pageURL)

	var m msg
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}

	select {
	case r := <-c.result:
		return r.token, r.err
	default:
		return "", errors.New("a verificação foi fechada antes de terminar")
	}
}

// solveCaptcha abre a janelinha num processo separado e espera a pessoa resolver.
func solveCaptcha(ctx context.Context, captcha *ProtonCaptcha) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	result := filepath.Join(os.TempDir(), fmt.Sprintf("DiscordGoLiveBypass-verificacao-%d.json", os.Getpid()))
	os.Remove(result)
	defer os.Remove(result)

	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-captcha", captcha.URL, "-challenge", captcha.Challenge, "-result", result)
	if err := cmd.Run(); err != nil && ctx.Err() != nil {
		return "", errors.New("a verificação do Proton demorou demais")
	}

	raw, err := os.ReadFile(result)
	if err != nil {
		return "", errors.New("a janela da verificação fechou sem resposta")
	}
	var answer struct {
		Token string `json:"token"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil {
		return "", err
	}
	if answer.Error != "" {
		return "", errors.New(answer.Error)
	}
	if !validCaptchaToken(answer.Token, captcha.Challenge) {
		return "", errors.New("a resposta da verificação não confere")
	}
	return answer.Token, nil
}

func writeCaptchaResult(path, token string, err error) int {
	answer := map[string]string{"token": token}
	if err != nil {
		answer = map[string]string{"error": err.Error()}
	}
	raw, _ := json.Marshal(answer)
	if path != "" {
		os.WriteFile(path, raw, 0o600)
	}
	if err != nil {
		return 1
	}
	return 0
}
