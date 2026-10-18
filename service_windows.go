//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
	"golang.zx2c4.com/wireguard/windows/driver"
)

const (
	serviceName        = "DiscordGoLiveBypass"
	serviceDisplayName = "DiscordGoLiveBypass"
	serviceDescription = "Mantém o túnel WireGuard que leva só o tráfego do Discord para fora do Brasil."

	serviceConfName = "wireguard.conf"
	maxLogBytes     = 1024 * 1024
)

func knownFolder(id *windows.KNOWNFOLDERID, fallbackEnv string) string {
	if path, err := windows.KnownFolderPath(id, 0); err == nil && path != "" {
		return path
	}
	return os.Getenv(fallbackEnv)
}

// O serviço roda como sistema, então o executável dele mora onde só administrador escreve.
// Rodar direto da pasta Downloads deixaria qualquer programa do usuário trocar o arquivo e
// ganhar o sistema inteiro na próxima vez que o serviço subisse.
func programDir() string {
	return filepath.Join(knownFolder(windows.FOLDERID_ProgramFiles, "ProgramFiles"), "DiscordGoLiveBypass")
}

func installedExe() string { return filepath.Join(programDir(), "DiscordGoLiveBypass.exe") }

func serviceDataDir() string {
	return filepath.Join(knownFolder(windows.FOLDERID_ProgramData, "ProgramData"), "DiscordGoLiveBypass")
}

func startMenuShortcut() string {
	return filepath.Join(knownFolder(windows.FOLDERID_CommonPrograms, "ProgramData"), "DiscordGoLiveBypass.lnk")
}

type serviceSettings struct {
	Enabled  bool   `json:"enabled"`
	OwnerSID string `json:"owner_sid,omitempty"`
	Mode     string `json:"mode,omitempty"`
}

func loadSettings(dir string) serviceSettings {
	var settings serviceSettings
	if raw, err := os.ReadFile(filepath.Join(dir, "settings.json")); err == nil {
		json.Unmarshal(raw, &settings)
	}
	return settings
}

func saveSettings(dir string, settings serviceSettings) error {
	raw, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, "settings.json"), raw)
}

func writeAtomic(path string, raw []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// A pasta guarda a chave da VPN. Só o sistema e os administradores entram nela, sem herdar
// nada do ProgramData, que por padrão deixa qualquer usuário ler.
func protectDataDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("D:PAI(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

type tunnelService struct {
	mu       sync.Mutex
	dir      string
	engine   *Engine
	settings serviceSettings
}

func (s *tunnelService) readConf() (string, bool) {
	raw, err := os.ReadFile(filepath.Join(s.dir, serviceConfName))
	if err != nil || len(raw) == 0 {
		return "", false
	}
	return string(raw), true
}

// O dono é quem instalou, e é no AppData dele que o Discord escreve o log.
func (s *tunnelService) start(text string) {
	s.engine.Start(text, effectiveMode(s.settings.Mode), discordLogPaths(s.settings.OwnerSID))
}

func (s *tunnelService) status() ServiceStatus {
	engine := s.engine.Status()
	status := ServiceStatus{
		Version:   version,
		Enabled:   s.settings.Enabled,
		State:     engine.State,
		Error:     engine.Error,
		Endpoint:  engine.Endpoint,
		Mode:      effectiveMode(s.settings.Mode),
		Routes:    engine.Routes,
		Streams:   engine.Streams,
		Handshake: -1,
		Rx:        engine.Rx,
		Tx:        engine.Tx,
	}
	if engine.Handshake >= 0 {
		status.Handshake = int64(engine.Handshake.Seconds())
	}
	if text, ok := s.readConf(); ok {
		status.HasConf = true
		if cfg, err := parseConf(text, "guardado"); err == nil {
			summary := summarize(cfg, text)
			status.Server = summary.Server
			if status.Endpoint == "" {
				status.Endpoint = summary.Endpoint
			}
		}
	}
	return status
}

func (s *tunnelService) handle(req ipcRequest) ipcResponse {
	s.mu.Lock()
	defer s.mu.Unlock()

	fail := func(err error) ipcResponse {
		return ipcResponse{Error: err.Error(), Status: s.status()}
	}

	switch req.Cmd {
	case cmdStatus:
	case cmdSetConf:
		if _, err := parseConf(req.Conf, "enviado"); err != nil {
			return fail(err)
		}
		if err := writeAtomic(filepath.Join(s.dir, serviceConfName), []byte(req.Conf)); err != nil {
			return fail(fmt.Errorf("não consegui guardar o arquivo da VPN: %v", err))
		}
		log.Printf("arquivo da VPN trocado")
		if s.settings.Enabled {
			s.start(req.Conf)
		}
	case cmdEnable:
		text, ok := s.readConf()
		if !ok {
			return fail(errors.New("falta o arquivo da VPN"))
		}
		s.settings.Enabled = true
		if err := saveSettings(s.dir, s.settings); err != nil {
			log.Printf("guardando a preferência: %v", err)
		}
		s.start(text)
	case cmdSetMode:
		if !validMode(req.Mode) {
			return fail(fmt.Errorf("modo desconhecido: %q", req.Mode))
		}
		s.settings.Mode = req.Mode
		if err := saveSettings(s.dir, s.settings); err != nil {
			return fail(fmt.Errorf("não consegui guardar o modo: %v", err))
		}
		log.Printf("modo trocado para %s", req.Mode)
		s.engine.SetMode(req.Mode)
	case cmdDisable:
		s.settings.Enabled = false
		if err := saveSettings(s.dir, s.settings); err != nil {
			log.Printf("guardando a preferência: %v", err)
		}
		s.engine.Stop()
	default:
		return fail(fmt.Errorf("pedido desconhecido: %q", req.Cmd))
	}
	return ipcResponse{OK: true, Status: s.status()}
}

func (s *tunnelService) Execute(args []string, requests <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	changes <- svc.Status{State: svc.StartPending}

	listener, err := listenIPC(s.settings.OwnerSID)
	if err != nil {
		log.Printf("não consegui abrir o canal da janela: %v", err)
		return true, 1
	}
	go serveIPC(listener, s.handle)

	if s.settings.Enabled {
		if text, ok := s.readConf(); ok {
			s.start(text)
		}
	}
	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for req := range requests {
		switch req.Cmd {
		case svc.Interrogate:
			changes <- req.CurrentStatus
		case svc.Stop, svc.Shutdown:
			changes <- svc.Status{State: svc.StopPending}
			listener.Close()
			s.engine.Stop()
			log.Printf("serviço parado")
			return false, 0
		}
	}
	return false, 0
}

func runService() error {
	dir := serviceDataDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	logPath := filepath.Join(dir, "tunnel.log")
	if info, err := os.Stat(logPath); err == nil && info.Size() > maxLogBytes {
		os.Rename(logPath, logPath+".1")
	}
	if file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
		defer file.Close()
		log.SetOutput(file)
	}
	log.SetFlags(log.Ldate | log.Ltime)
	log.Printf("serviço iniciando, versão %s", version)

	s := &tunnelService{dir: dir, engine: NewEngine(), settings: loadSettings(dir)}
	return svc.Run(serviceName, s)
}

func waitServiceState(service *mgr.Service, want svc.State, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		status, err := service.Query()
		if err != nil {
			return err
		}
		if status.State == want {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("o serviço não chegou ao estado esperado a tempo")
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func stopService(service *mgr.Service) {
	if status, err := service.Query(); err == nil && status.State != svc.Stopped {
		service.Control(svc.Stop)
		waitServiceState(service, svc.Stopped, 30*time.Second)
	}
}

// Um executável em uso não pode ser sobrescrito, mas pode ser renomeado. A janela antiga
// pode estar aberta a partir da pasta instalada justamente durante uma atualização.
func replaceFile(src, dest string) error {
	if same, _ := sameFile(src, dest); same {
		return nil
	}
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	old := dest + ".antigo"
	os.Remove(old)
	if _, err := os.Stat(dest); err == nil {
		if err := os.Rename(dest, old); err != nil {
			return err
		}
	}
	if err := os.WriteFile(dest, raw, 0o755); err != nil {
		os.Rename(old, dest)
		return err
	}
	if err := os.Remove(old); err != nil {
		deleteOnReboot(old)
	}
	return nil
}

func deleteOnReboot(path string) {
	if p, err := windows.UTF16PtrFromString(path); err == nil {
		windows.MoveFileEx(p, nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
	}
}

func hiddenCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	return cmd
}

func createShortcut(link, target string) error {
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	script := fmt.Sprintf("$s = (New-Object -ComObject WScript.Shell).CreateShortcut(%s); $s.TargetPath = %s; $s.WorkingDirectory = %s; $s.Description = 'Go Live de volta no Discord'; $s.Save()",
		quote(link), quote(target), quote(filepath.Dir(target)))
	out, err := hiddenCommand("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// installService copia o executável para Arquivos de Programas, registra o serviço e o
// atalho no menu Iniciar. Serve também de atualização: rodado por uma versão mais nova,
// troca o executável e reinicia o serviço com a configuração que já estava guardada.
func installService(ownerSID string) error {
	if !isElevated() {
		return errors.New("a instalação precisa de permissão de administrador")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	manager, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("não consegui falar com o gerenciador de serviços: %v", err)
	}
	defer manager.Disconnect()

	existing, err := manager.OpenService(serviceName)
	if err == nil {
		stopService(existing)
	}

	if err := os.MkdirAll(programDir(), 0o755); err != nil {
		return fmt.Errorf("não consegui criar %s: %v", programDir(), err)
	}
	if err := replaceFile(exe, installedExe()); err != nil {
		return fmt.Errorf("não consegui copiar o programa para %s: %v", programDir(), err)
	}

	dir := serviceDataDir()
	if err := protectDataDir(dir); err != nil {
		return fmt.Errorf("não consegui preparar %s: %v", dir, err)
	}
	settings := loadSettings(dir)
	if ownerSID != "" {
		settings.OwnerSID = ownerSID
	}
	if err := saveSettings(dir, settings); err != nil {
		return err
	}

	config := mgr.Config{
		ServiceType:  windows.SERVICE_WIN32_OWN_PROCESS,
		StartType:    mgr.StartAutomatic,
		ErrorControl: mgr.ErrorNormal,
		DisplayName:  serviceDisplayName,
		Description:  serviceDescription,
	}
	service := existing
	if service == nil {
		service, err = manager.CreateService(serviceName, installedExe(), config, "-service")
		if err != nil {
			return fmt.Errorf("não consegui registrar o serviço: %v", err)
		}
	} else {
		current, err := service.Config()
		if err != nil {
			return err
		}
		current.BinaryPathName = fmt.Sprintf("%q -service", installedExe())
		current.StartType = config.StartType
		current.DisplayName = config.DisplayName
		current.Description = config.Description
		if err := service.UpdateConfig(current); err != nil {
			return fmt.Errorf("não consegui atualizar o serviço: %v", err)
		}
	}
	defer service.Close()

	// Se o serviço cair, o Windows sobe de novo: sem ele o Discord perde o túnel até a
	// próxima vez que a pessoa abrir a janela.
	service.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 15 * time.Second},
		{Type: mgr.ServiceRestart, Delay: time.Minute},
	}, uint32((24 * time.Hour).Seconds()))

	if err := service.Start(); err != nil {
		return fmt.Errorf("o serviço foi instalado mas não iniciou: %v", err)
	}
	if err := waitServiceState(service, svc.Running, 20*time.Second); err != nil {
		return fmt.Errorf("o serviço foi instalado mas não ficou de pé: %v", err)
	}

	if err := createShortcut(startMenuShortcut(), installedExe()); err != nil {
		log.Printf("atalho do menu Iniciar: %v", err)
	}
	return nil
}

// uninstallService desfaz tudo o que a instalação fez. Ele roda de uma cópia temporária,
// porque o executável instalado não pode apagar a si mesmo, e espera a janela fechar.
func uninstallService(waitPID int) error {
	if !isElevated() {
		return errors.New("a desinstalação precisa de permissão de administrador")
	}
	if waitPID > 0 {
		waitProcessExit(waitPID, 30*time.Second)
	}

	manager, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("não consegui falar com o gerenciador de serviços: %v", err)
	}
	defer manager.Disconnect()

	if service, err := manager.OpenService(serviceName); err == nil {
		stopService(service)
		if err := service.Delete(); err != nil {
			service.Close()
			return fmt.Errorf("não consegui remover o serviço: %v", err)
		}
		service.Close()
	}

	// O serviço já fechou o adaptador. Sem nenhum outro em uso, o driver sai junto.
	func() {
		defer func() { recover() }()
		driver.Uninstall()
	}()

	os.Remove(startMenuShortcut())
	if err := os.RemoveAll(serviceDataDir()); err != nil {
		log.Printf("dados do serviço: %v", err)
	}
	// O que ainda estiver em uso sai na próxima reinicialização. O Windows só apaga uma
	// pasta vazia, então os arquivos vão para a fila antes dela.
	if err := os.RemoveAll(programDir()); err != nil {
		var leftovers []string
		filepath.Walk(programDir(), func(path string, info os.FileInfo, err error) error {
			if err == nil {
				leftovers = append(leftovers, path)
			}
			return nil
		})
		for i := len(leftovers) - 1; i >= 0; i-- {
			deleteOnReboot(leftovers[i])
		}
	}

	if self, err := os.Executable(); err == nil {
		deleteOnReboot(self)
	}
	return nil
}

func waitProcessExit(pid int, timeout time.Duration) {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(handle)
	windows.WaitForSingleObject(handle, uint32(timeout.Milliseconds()))
}

// O processo elevado não tem como devolver texto para a janela além do código de saída,
// então o motivo de uma falha vai num arquivo que ela lê depois.
func writeResult(path string, err error) int {
	if path == "" {
		if err != nil {
			return 1
		}
		return 0
	}
	message := ""
	if err != nil {
		message = err.Error()
	}
	raw, _ := json.Marshal(map[string]string{"error": message})
	os.WriteFile(path, raw, 0o600)
	if err != nil {
		return 1
	}
	return 0
}

func readResult(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	var result map[string]string
	if err := json.NewDecoder(io.LimitReader(file, 64*1024)).Decode(&result); err != nil {
		return nil
	}
	if result["error"] != "" {
		return errors.New(result["error"])
	}
	return nil
}
