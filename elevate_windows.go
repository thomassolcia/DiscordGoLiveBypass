//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procShellExecuteEx = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")

// SHELLEXECUTEINFOW. O x/sys só expõe o ShellExecute simples, que não devolve o processo
// criado, e sem ele não dá para esperar a instalação terminar nem ler o resultado.
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
)

var errElevationDenied = errors.New("a permissão de administrador foi negada")

func isElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

// runElevated roda o executável com administrador, sem janela, e espera ele terminar. É o
// único momento em que o Windows pergunta: na instalação, na atualização e na remoção.
func runElevated(exe string, args []string) (uint32, error) {
	process, err := shellRunas(exe, args)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(process)

	if _, err := windows.WaitForSingleObject(process, windows.INFINITE); err != nil {
		return 0, err
	}
	var code uint32
	if err := windows.GetExitCodeProcess(process, &code); err != nil {
		return 0, err
	}
	return code, nil
}

// startElevated não espera: a remoção precisa que a janela feche para apagar o executável.
func startElevated(exe string, args []string) error {
	process, err := shellRunas(exe, args)
	if err != nil {
		return err
	}
	return windows.CloseHandle(process)
}

func shellRunas(exe string, args []string) (windows.Handle, error) {
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
		return 0, errors.New("o Windows não devolveu o processo elevado")
	}
	return info.hProcess, nil
}

func currentUserSID() string {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return ""
	}
	return user.User.Sid.String()
}

func eachProcess(visit func(entry *windows.ProcessEntry32) bool) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if !visit(&entry) {
			return
		}
	}
}

// Lista de processos pela API, e não pelo tasklist: chamado de um programa com janela, cada
// tasklist abriria um console piscando na tela.
func processRunning(name string) bool {
	found := false
	eachProcess(func(entry *windows.ProcessEntry32) bool {
		if strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), name) {
			found = true
			return false
		}
		return true
	})
	return found
}

// Todos os processos do Discord têm o mesmo nome, da janela aos auxiliares, então fechar
// pelo nome derruba a árvore inteira, como o taskkill /T fazia.
func terminateAll(name string) error {
	var pids []uint32
	eachProcess(func(entry *windows.ProcessEntry32) bool {
		if strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), name) {
			pids = append(pids, entry.ProcessID)
		}
		return true
	})

	var firstErr error
	for _, pid := range pids {
		handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
		if err == nil {
			err = windows.TerminateProcess(handle, 1)
			windows.CloseHandle(handle)
		}
		if err != nil && firstErr == nil && !errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			firstErr = err
		}
	}
	return firstErr
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
