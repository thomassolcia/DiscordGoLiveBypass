//go:build windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procShellExecuteEx = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")

// SHELLEXECUTEINFOW. O x/sys só expõe o ShellExecute simples, que não devolve o processo
// criado, e sem ele não dá para saber se o túnel morreu antes de ficar pronto.
type shellExecuteInfo struct {
	cbSize         uint32
	fMask          uint32
	hwnd           windows.Handle
	lpVerb         *uint16
	lpFile         *uint16
	lpParameters   *uint16
	lpDirectory    *uint16
	nShow          int32
	hInstApp       windows.Handle
	lpIDList       uintptr
	lpClass        *uint16
	hkeyClass      windows.Handle
	dwHotKey       uint32
	hIconOrMonitor windows.Handle
	hProcess       windows.Handle
}

const (
	seeMaskNoCloseProcess = 0x00000040
	seeMaskNoAsync        = 0x00000100
	seeMaskFlagNoUI       = 0x00000400

	stillActive = 259
)

func isElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		// Acesso negado vem de um processo que existe mas é de outro usuário.
		return errors.Is(err, windows.ERROR_ACCESS_DENIED)
	}
	defer windows.CloseHandle(handle)

	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	return code == stillActive
}

func processImageBase(pid int) string {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(handle)

	buf := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(handle, 0, &buf[0], &size); err != nil {
		return ""
	}
	return filepath.Base(windows.UTF16ToString(buf[:size]))
}

// O PID gravado no estado pode ter sido reciclado pelo Windows depois de um túnel que
// morreu sem avisar. Só conta como túnel vivo se ainda for este mesmo programa.
func helperAlive(pid int) bool {
	if !processAlive(pid) {
		return false
	}
	exe, err := os.Executable()
	return err == nil && strings.EqualFold(processImageBase(pid), filepath.Base(exe))
}

func terminateProcess(pid int) error {
	handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	return windows.TerminateProcess(handle, 1)
}

// Lista de processos pela API, e não pelo tasklist: o túnel pergunta isso a cada poucos
// segundos durante horas, e abrir um processo novo toda vez seria desperdício.
func processRunning(name string) bool {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), name) {
			return true
		}
	}
	return false
}

// Sobe o processo do túnel com elevação. Quem já é administrador não precisa do UAC, mas
// continua precisando de um processo separado, que sobrevive à janela do launcher fechar.
func startTunnelHelper(args []string) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, err
	}

	if isElevated() {
		cmd := exec.Command(exe, args...)
		cmd.Dir = filepath.Dir(exe)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    true,
			CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
		}
		if err := cmd.Start(); err != nil {
			return 0, err
		}
		pid := cmd.Process.Pid
		return pid, cmd.Process.Release()
	}

	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = windows.EscapeArg(arg)
	}

	verb, _ := windows.UTF16PtrFromString("runas")
	file, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return 0, err
	}
	params, err := windows.UTF16PtrFromString(strings.Join(quoted, " "))
	if err != nil {
		return 0, err
	}
	dir, _ := windows.UTF16PtrFromString(filepath.Dir(exe))

	info := shellExecuteInfo{
		fMask:        seeMaskNoCloseProcess | seeMaskNoAsync | seeMaskFlagNoUI,
		lpVerb:       verb,
		lpFile:       file,
		lpParameters: params,
		lpDirectory:  dir,
		nShow:        windows.SW_HIDE,
	}
	info.cbSize = uint32(unsafe.Sizeof(info))

	ok, _, callErr := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		if errors.Is(callErr, windows.ERROR_CANCELLED) {
			return 0, errElevationDenied
		}
		return 0, callErr
	}
	if info.hProcess == 0 {
		return 0, errors.New("o Windows nao devolveu o processo do tunel")
	}
	defer windows.CloseHandle(info.hProcess)

	pid, err := windows.GetProcessId(info.hProcess)
	if err != nil {
		return 0, err
	}
	return int(pid), nil
}

// Um launcher rodando como administrador abriria o Discord como administrador também. O
// Explorer abre como o usuário da sessão, que é como o Discord deve rodar.
func launchUnelevated(binary string) error {
	explorer := filepath.Join(os.Getenv("WINDIR"), "explorer.exe")
	return exec.Command(explorer, binary).Start()
}

func openBrowser(url string) error {
	verb, _ := windows.UTF16PtrFromString("open")
	target, err := windows.UTF16PtrFromString(url)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, target, nil, nil, windows.SW_SHOWNORMAL)
}

// A pasta Downloads pode ter sido movida para outro disco, então o caminho vem do próprio
// Windows e o padrão em %USERPROFILE% fica só de reserva.
func downloadsDir() string {
	if path, err := windows.KnownFolderPath(windows.FOLDERID_Downloads, 0); err == nil && path != "" {
		return path
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Downloads")
}
