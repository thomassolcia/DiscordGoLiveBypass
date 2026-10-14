//go:build windows

package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	winopts "github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

//go:embed all:frontend
var frontendFiles embed.FS

const (
	protonDownloads = "https://account.protonvpn.com/downloads"
	helpURL         = "https://github.com/thomassolcia/DiscordGoLiveBypass#faq"

	themeSystem = "sistema"
	themeLight  = "claro"
	themeDark   = "escuro"
)

// App é a ponte da página com o Go: cada método exportado vira uma função em
// window.go.main.App, e o que ele devolve chega à página como JSON.
type App struct {
	ctx  context.Context
	tray *tray

	mu       sync.Mutex
	last     AppState
	hidden   bool
	quitting bool
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
	Discord        string         `json:"discord"`
	Error          string         `json:"error,omitempty"`
}

type ExitResult struct {
	Country string `json:"country"`
	Label   string `json:"label"`
	IP      string `json:"ip"`
	Ms      int64  `json:"ms"`
	Blocked bool   `json:"blocked"`
}

// Prefs são escolhas da pessoa, não do túnel: ficam na pasta dela e não no serviço.
type Prefs struct {
	Theme       string `json:"tema"`
	OpenDiscord bool   `json:"abrir_discord"`
	// O que o X faz: perguntar, ir para a bandeja ou desligar o túnel e fechar.
	CloseAction string `json:"ao_fechar"`
}

const (
	closeAsk  = "perguntar"
	closeTray = "bandeja"
	closeStop = "parar"
)

func prefsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "DiscordGoLiveBypass", "janela.json")
}

func loadPrefs() Prefs {
	prefs := Prefs{Theme: themeSystem, OpenDiscord: true, CloseAction: closeAsk}
	if raw, err := os.ReadFile(prefsPath()); err == nil {
		json.Unmarshal(raw, &prefs)
	}
	switch prefs.Theme {
	case themeSystem, themeLight, themeDark:
	default:
		prefs.Theme = themeSystem
	}
	switch prefs.CloseAction {
	case closeAsk, closeTray, closeStop:
	default:
		prefs.CloseAction = closeAsk
	}
	return prefs
}

func savePrefs(prefs Prefs) error {
	path := prefsPath()
	if path == "" {
		return errors.New("não achei a pasta de configurações do usuário")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(prefs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

// O tema "sistema" segue a escolha do Windows para os aplicativos, a mesma que o
// Explorador usa.
func systemPrefersLight() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	value, _, err := key.GetIntegerValue("AppsUseLightTheme")
	return err == nil && value == 1
}

func resolvedTheme(theme string) string {
	if theme == themeSystem {
		if systemPrefersLight() {
			return themeLight
		}
		return themeDark
	}
	return theme
}

// Fundo da janela antes da página carregar. Com a cor errada, quem usa o tema claro veria
// um clarão escuro a cada abertura.
func backgroundFor(theme string) *options.RGBA {
	if resolvedTheme(theme) == themeLight {
		return &options.RGBA{R: 242, G: 243, B: 245, A: 255}
	}
	return &options.RGBA{R: 26, G: 27, B: 30, A: 255}
}

func (a *App) applyTheme(theme string) {
	switch theme {
	case themeLight:
		runtime.WindowSetLightTheme(a.ctx)
	case themeDark:
		runtime.WindowSetDarkTheme(a.ctx)
	default:
		runtime.WindowSetSystemDefaultTheme(a.ctx)
	}
	bg := backgroundFor(theme)
	runtime.WindowSetBackgroundColour(a.ctx, bg.R, bg.G, bg.B, bg.A)
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	runtime.OnFileDrop(ctx, func(x, y int, paths []string) {
		if len(paths) > 0 {
			runtime.EventsEmit(ctx, "arquivo-solto", a.LerArquivo(paths[0]))
		}
	})

	tray, err := startTray("DiscordGoLiveBypass", a.MostrarJanela, a.trayMenu)
	if err != nil {
		log.Printf("sem ícone na bandeja, minimizar fica como no Windows: %v", err)
	} else {
		log.Printf("ícone na bandeja pronto")
	}
	a.tray = tray
	go a.watchMinimize()
}

func (a *App) shutdown(ctx context.Context) {
	a.tray.Close()
}

// Minimizar manda a janela para a bandeja. O Wails não avisa quando ela é minimizada, então
// a janela é conferida em intervalos curtos, o que custa quase nada.
func (a *App) watchMinimize() {
	if a.tray == nil {
		return
	}
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		a.mu.Lock()
		hidden := a.hidden
		a.mu.Unlock()
		if !hidden && runtime.WindowIsMinimised(a.ctx) {
			runtime.WindowHide(a.ctx)
			a.mu.Lock()
			a.hidden = true
			a.mu.Unlock()
		}
	}
}

func (a *App) MostrarJanela() {
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
	a.mu.Lock()
	a.hidden = false
	a.mu.Unlock()
}

func (a *App) hideToTray() {
	runtime.WindowHide(a.ctx)
	a.mu.Lock()
	a.hidden = true
	a.mu.Unlock()
}

func (a *App) quit() {
	a.mu.Lock()
	a.quitting = true
	a.mu.Unlock()
	runtime.Quit(a.ctx)
}

func configured(state AppState) bool {
	return state.ServiceRunning && state.Status != nil && state.Status.HasConf
}

// beforeClose decide o que o X faz. Sem túnel configurado não há o que perguntar; com
// ele, a página pergunta se a pessoa quer deixar o app na bandeja ou desligar o túnel, a
// menos que ela já tenha escolhido de vez.
func (a *App) beforeClose(ctx context.Context) bool {
	a.mu.Lock()
	quitting, state, tray := a.quitting, a.last, a.tray
	a.mu.Unlock()
	if quitting || !configured(state) {
		return false
	}

	switch loadPrefs().CloseAction {
	case closeTray:
		if tray != nil {
			a.hideToTray()
			return true
		}
	case closeStop:
		a.Desligar()
		return false
	}
	runtime.EventsEmit(ctx, "pedido-fechar", tray != nil)
	return true
}

// Respostas da pergunta do X. Com "lembrar", a escolha vira o padrão e a pergunta some;
// ela pode ser trocada de novo nas preferências.
func (a *App) FecharParaBandeja(lembrar bool) {
	if lembrar {
		prefs := loadPrefs()
		prefs.CloseAction = closeTray
		savePrefs(prefs)
	}
	a.hideToTray()
}

func (a *App) PararEFechar(lembrar bool) error {
	if lembrar {
		prefs := loadPrefs()
		prefs.CloseAction = closeStop
		savePrefs(prefs)
	}
	if _, err := callService(ipcRequest{Cmd: cmdDisable}, 30*time.Second); err != nil {
		return err
	}
	a.quit()
	return nil
}

// Sair, no menu da bandeja, faz a mesma pergunta que o X, com a janela à mostra.
func (a *App) Sair() {
	a.MostrarJanela()
	if !a.beforeClose(a.ctx) {
		a.quit()
	}
}

func stateLabel(state AppState) string {
	if state.Status == nil || !state.Status.HasConf {
		return "não configurado"
	}
	switch state.Status.State {
	case stateOn:
		return "Go Live liberado"
	case stateConnecting:
		return "conectando"
	case stateNoServer:
		return "servidor sem resposta"
	case stateOff:
		return "túnel desligado"
	}
	return "erro no túnel"
}

func (a *App) trayMenu() []trayItem {
	a.mu.Lock()
	state := a.last
	a.mu.Unlock()

	items := []trayItem{{Label: "Abrir o painel", Action: a.MostrarJanela, Default: true}}
	if state.Discord != "" {
		items = append(items, trayItem{Label: "Abrir o " + state.Discord, Action: func() { a.AbrirDiscord() }})
	}
	if state.Status != nil && state.Status.HasConf {
		items = append(items, trayItem{})
		if state.Status.Enabled {
			items = append(items, trayItem{Label: "Desligar o túnel", Action: func() { a.Desligar(); a.notifyPage() }})
		} else {
			items = append(items, trayItem{Label: "Ligar o túnel", Action: func() { a.Ligar(); a.notifyPage() }})
		}
	}
	return append(items, trayItem{}, trayItem{Label: "Sair", Action: a.Sair})
}

func (a *App) notifyPage() { runtime.EventsEmit(a.ctx, "atualizar") }

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

// O Discord que "Abrir o Discord" abre: o que já está aberto, ou o primeiro instalado.
func discordToOpen() (Channel, string, error) {
	if running := RunningChannels(); len(running) > 0 {
		binary, err := LocateDiscord(running[0])
		return running[0], binary, err
	}
	return DetectChannel()
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
	if channel, _, err := discordToOpen(); err == nil {
		state.Discord = channel.Friendly
	}

	a.mu.Lock()
	a.last = state
	a.mu.Unlock()
	if a.tray != nil {
		a.tray.SetTip("DiscordGoLiveBypass: " + stateLabel(state))
	}
	return state
}

func (a *App) Preferencias() Prefs { return loadPrefs() }

func (a *App) SalvarPreferencias(prefs Prefs) (Prefs, error) {
	if err := savePrefs(prefs); err != nil {
		return loadPrefs(), err
	}
	prefs = loadPrefs()
	a.applyTheme(prefs.Theme)
	return prefs, nil
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

// Com o túnel sempre ligado, abrir o Discord é só abrir: ele já nasce passando pelo túnel.
// Se ele já estiver aberto, abrir de novo traz a janela dele para a frente.
func (a *App) AbrirDiscord() (string, error) {
	channel, binary, err := discordToOpen()
	if err != nil {
		return "", errors.New("não achei o Discord instalado neste computador")
	}
	if _, err := Launch(binary, nil); err != nil {
		return "", fmt.Errorf("não consegui abrir o %s: %v", channel.Friendly, err)
	}
	return channel.Friendly, nil
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

// O registro da janela fica na pasta do usuário, ao lado das preferências: é o que dá para
// pedir num relato quando algo da janela falha, já que ela não tem console.
func openGUILog() {
	path := prefsPath()
	if path == "" {
		return
	}
	logPath := filepath.Join(filepath.Dir(path), "janela.log")
	os.MkdirAll(filepath.Dir(logPath), 0o700)
	if info, err := os.Stat(logPath); err == nil && info.Size() > maxLogBytes {
		os.Rename(logPath, logPath+".1")
	}
	if file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
		log.SetOutput(file)
		log.SetFlags(log.Ldate | log.Ltime)
	}
}

func runGUI() error {
	openGUILog()
	log.Printf("janela iniciando, versão %s", version)
	app := &App{}
	assets, err := fs.Sub(frontendFiles, "frontend")
	if err != nil {
		return err
	}

	prefs := loadPrefs()
	theme := winopts.SystemDefault
	switch prefs.Theme {
	case themeLight:
		theme = winopts.Light
	case themeDark:
		theme = winopts.Dark
	}

	return wails.Run(&options.App{
		Title:            "DiscordGoLiveBypass",
		Width:            940,
		Height:           700,
		MinWidth:         800,
		MinHeight:        600,
		BackgroundColour: backgroundFor(prefs.Theme),
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		OnBeforeClose:    app.beforeClose,
		OnShutdown:       app.shutdown,
		Bind:             []interface{}{app},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "c6b0c3d6-0f63-4d64-9d1c-discordgolivebypass",
			OnSecondInstanceLaunch: func(options.SecondInstanceData) { app.MostrarJanela() },
		},
		DragAndDrop: &options.DragAndDrop{EnableFileDrop: true, DisableWebViewDrop: true},
		Windows:     &winopts.Options{Theme: theme},
	})
}

func messageBox(title, text string, icon uint32) {
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(text)
	windows.MessageBox(0, m, t, windows.MB_OK|icon)
}
