//go:build windows

package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	winopts "github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
)

//go:embed all:frontend
var frontendFiles embed.FS

const (
	protonDownloads = "https://account.protonvpn.com/downloads"
	helpURL         = "https://github.com/thomassolcia/DiscordGoLiveBypass#faq"
)

// App é a ponte da página com o Go: cada método exportado vira uma função em
// window.go.main.App, e o que ele devolve chega à página como JSON.
type App struct {
	ctx context.Context
}

type ConfInfo struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Server   string `json:"server"`
	Endpoint string `json:"endpoint"`
	Modified string `json:"modified"`
	Origin   string `json:"origin"`
	Error    string `json:"error,omitempty"`
}

type AppState struct {
	Version        string         `json:"version"`
	Installed      bool           `json:"installed"`
	ServiceRunning bool           `json:"service_running"`
	Status         *ServiceStatus `json:"status,omitempty"`
	NeedsUpdate    bool           `json:"needs_update"`
	DiscordOpen    []string       `json:"discord_open"`
	Error          string         `json:"error,omitempty"`
}

type ExitResult struct {
	Country string `json:"country"`
	Label   string `json:"label"`
	IP      string `json:"ip"`
	Ms      int64  `json:"ms"`
	Blocked bool   `json:"blocked"`
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	runtime.OnFileDrop(ctx, func(x, y int, paths []string) {
		if len(paths) > 0 {
			runtime.EventsEmit(ctx, "arquivo-solto", a.LerArquivo(paths[0]))
		}
	})
}

// Consulta com o mínimo de permissão: qualquer usuário pode perguntar se o serviço existe
// e se está rodando, sem administrador.
func serviceInstalled() (installed, running bool) {
	manager, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return false, false
	}
	defer windows.CloseServiceHandle(manager)

	name, _ := windows.UTF16PtrFromString(serviceName)
	service, err := windows.OpenService(manager, name, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return false, false
	}
	defer windows.CloseServiceHandle(service)

	var status windows.SERVICE_STATUS
	if err := windows.QueryServiceStatus(service, &status); err != nil {
		return true, false
	}
	return true, status.CurrentState == windows.SERVICE_RUNNING
}

func (a *App) Estado() AppState {
	state := AppState{Version: version, DiscordOpen: []string{}}
	state.Installed, state.ServiceRunning = serviceInstalled()
	if state.ServiceRunning {
		resp, err := callService(ipcRequest{Cmd: cmdStatus}, 5*time.Second)
		if err != nil {
			state.Error = err.Error()
		} else {
			status := resp.Status
			state.Status = &status
			state.NeedsUpdate = newerThan(parseVersion(version), parseVersion(status.Version))
		}
	}
	for _, channel := range RunningChannels() {
		state.DiscordOpen = append(state.DiscordOpen, channel.Friendly)
	}
	return state
}

func (a *App) LerArquivo(path string) ConfInfo {
	info := ConfInfo{Path: path, Name: filepath.Base(path), Origin: "escolhido"}
	if stat, err := os.Stat(path); err == nil {
		info.Modified = stat.ModTime().Format("02/01 15:04")
	}
	summary, err := ValidateConf(path)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	info.Server, info.Endpoint = summary.Server, summary.Endpoint
	return info
}

// Candidatos são os arquivos que a pessoa provavelmente quer: o que a 2.0 guardou e os
// .conf de WireGuard recém-baixados. Só entram os que passam na validação.
func (a *App) Candidatos() []ConfInfo {
	list := []ConfInfo{}
	seen := map[string]bool{}
	add := func(path, origin string) {
		key := strings.ToLower(path)
		if seen[key] {
			return
		}
		seen[key] = true
		if info := a.LerArquivo(path); info.Error == "" {
			info.Origin = origin
			list = append(list, info)
		}
	}

	if old, err := storedConfPath(); err == nil {
		if _, err := os.Stat(old); err == nil {
			add(old, "2.0")
		}
	}
	for _, candidate := range findConfCandidates([]string{downloadsDir(), exeDir()}) {
		add(candidate.Path, "downloads")
	}
	return list
}

func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(exe)
}

func (a *App) EscolherArquivo() ConfInfo {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            "Escolha o arquivo da VPN",
		DefaultDirectory: downloadsDir(),
		Filters:          []runtime.FileFilter{{DisplayName: "Configuração WireGuard (*.conf)", Pattern: "*.conf"}},
	})
	if err != nil || path == "" {
		return ConfInfo{}
	}
	return a.LerArquivo(path)
}

// O serviço acabou de subir e o canal dele leva um instante para aparecer.
func waitService(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		_, err := callService(ipcRequest{Cmd: cmdStatus}, 5*time.Second)
		if err == nil || !errors.Is(err, errServiceAbsent) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func (a *App) install() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	result := filepath.Join(os.TempDir(), fmt.Sprintf("DiscordGoLiveBypass-instalacao-%d.json", os.Getpid()))
	os.Remove(result)
	defer os.Remove(result)

	code, err := runElevated(exe, []string{"-install", "-owner", currentUserSID(), "-result", result})
	if errors.Is(err, errElevationDenied) {
		return errors.New("sem a permissão de administrador o Windows não deixa instalar o serviço do túnel. Clique de novo e responda Sim no aviso")
	}
	if err != nil {
		return fmt.Errorf("não consegui pedir a permissão de administrador: %v", err)
	}
	if err := readResult(result); err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("a instalação terminou com o código %d", code)
	}
	return waitService(15 * time.Second)
}

func (a *App) sendConf(path string) error {
	if path == "" {
		return nil
	}
	text, err := readConfFile(path)
	if err != nil {
		return err
	}
	_, err = callService(ipcRequest{Cmd: cmdSetConf, Conf: text}, 30*time.Second)
	return err
}

// Ativar instala o serviço na primeira vez, ou quando esta janela é de uma versão mais nova
// que a instalada, entrega o arquivo da VPN e liga o túnel.
func (a *App) Ativar(path string) (AppState, error) {
	state := a.Estado()
	if !state.ServiceRunning || state.NeedsUpdate {
		if err := a.install(); err != nil {
			return a.Estado(), err
		}
	}
	if err := a.sendConf(path); err != nil {
		return a.Estado(), err
	}
	if _, err := callService(ipcRequest{Cmd: cmdEnable}, 30*time.Second); err != nil {
		return a.Estado(), err
	}
	return a.Estado(), nil
}

func (a *App) Ligar() (AppState, error) {
	_, err := callService(ipcRequest{Cmd: cmdEnable}, 30*time.Second)
	return a.Estado(), err
}

func (a *App) Desligar() (AppState, error) {
	_, err := callService(ipcRequest{Cmd: cmdDisable}, 30*time.Second)
	return a.Estado(), err
}

func (a *App) TrocarArquivo(path string) (AppState, error) {
	err := a.sendConf(path)
	return a.Estado(), err
}

func (a *App) Atualizar() (AppState, error) {
	err := a.install()
	return a.Estado(), err
}

func (a *App) ConferirSaida() (ExitResult, error) {
	exit, err := CheckExit(10 * time.Second)
	if err != nil {
		return ExitResult{}, errors.New(DescribeNetwork(err))
	}
	return ExitResult{
		Country: exit.Country,
		Label:   CountryLabel(exit.Country),
		IP:      exit.IP,
		Ms:      exit.Latency.Milliseconds(),
		Blocked: exit.Country == "BR",
	}, nil
}

// O Discord aberto antes do túnel mantém as conexões que já tinha pela rota antiga até
// reconectar sozinho. Reiniciar faz tudo passar pelo túnel na hora.
func (a *App) ReiniciarDiscord() ([]string, error) {
	var restarted []string
	for _, channel := range RunningChannels() {
		binary, err := LocateDiscord(channel)
		if err != nil {
			return restarted, err
		}
		if err := Terminate(channel); err != nil {
			return restarted, fmt.Errorf("não consegui fechar o %s: %v", channel.Friendly, err)
		}
		for i := 0; i < 25 && IsRunning(channel); i++ {
			time.Sleep(200 * time.Millisecond)
		}
		// O Windows demora um pouco para soltar a trava de instância única. Abrir em cima
		// disso faz o processo novo se achar duplicado e sair sozinho.
		time.Sleep(2 * time.Second)
		if _, err := Launch(binary, nil); err != nil {
			return restarted, fmt.Errorf("não consegui abrir o %s: %v", channel.Friendly, err)
		}
		restarted = append(restarted, channel.Friendly)
	}
	return restarted, nil
}

func (a *App) VersaoNova() string { return NewerRelease() }

func (a *App) AbrirProton()  { runtime.BrowserOpenURL(a.ctx, protonDownloads) }
func (a *App) AbrirAjuda()   { runtime.BrowserOpenURL(a.ctx, helpURL) }
func (a *App) AbrirRelease() { runtime.BrowserOpenURL(a.ctx, releasesPage) }

// O executável instalado não pode apagar a si mesmo, então a remoção roda de uma cópia
// temporária, que espera esta janela fechar antes de começar.
func (a *App) Desinstalar() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	copyPath := filepath.Join(os.TempDir(), "DiscordGoLiveBypass-remover.exe")
	raw, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	if err := os.WriteFile(copyPath, raw, 0o755); err != nil {
		return err
	}

	err = startElevated(copyPath, []string{"-uninstall", "-wait-pid", fmt.Sprint(os.Getpid())})
	if errors.Is(err, errElevationDenied) {
		return errors.New("sem a permissão de administrador o Windows não deixa remover o serviço")
	}
	if err != nil {
		return err
	}
	runtime.Quit(a.ctx)
	return nil
}

func runGUI() error {
	app := &App{}
	assets, err := fs.Sub(frontendFiles, "frontend")
	if err != nil {
		return err
	}
	return wails.Run(&options.App{
		Title:            "DiscordGoLiveBypass",
		Width:            460,
		Height:           700,
		MinWidth:         420,
		MinHeight:        560,
		BackgroundColour: &options.RGBA{R: 30, G: 31, B: 34, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		Bind:             []interface{}{app},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "c6b0c3d6-0f63-4d64-9d1c-discordgolivebypass",
			OnSecondInstanceLaunch: func(options.SecondInstanceData) {
				runtime.WindowUnminimise(app.ctx)
				runtime.WindowShow(app.ctx)
			},
		},
		DragAndDrop: &options.DragAndDrop{EnableFileDrop: true, DisableWebViewDrop: true},
		Windows:     &winopts.Options{Theme: winopts.Dark},
	})
}

func messageBox(title, text string, icon uint32) {
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(text)
	windows.MessageBox(0, m, t, windows.MB_OK|icon)
}
