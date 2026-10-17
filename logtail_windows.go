//go:build windows

package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	// Discord fechado ou canal não instalado: o arquivo é procurado de novo com essa folga.
	logRetryInterval = 2 * time.Second
	// De quanto em quanto tempo confere se o Discord trocou o arquivo do lugar.
	logRotateCheck = 500 * time.Millisecond
)

// logTail acompanha um log que o Discord escreve e, perto de 10 MB, renomeia para
// renderer_js.old.log antes de começar outro. O arquivo é aberto com FILE_SHARE_DELETE:
// sem ele, o os.Open do Go impediria essa troca enquanto o serviço estivesse lendo.
type logTail struct {
	path       string
	file       *os.File
	id         fileIdentity
	lastOffset int64
	buf        lineBuffer
	chunk      []byte
	opened     bool
	nextTry    time.Time
	nextCheck  time.Time
}

type fileIdentity struct {
	volume, high, low uint32
}

func newLogTail(path string) *logTail {
	return &logTail{path: path, chunk: make([]byte, 32*1024)}
}

func openShared(path string, access uint32) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return windows.InvalidHandle, err
	}
	return windows.CreateFile(name, access,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
}

func handleIdentity(handle windows.Handle) (fileIdentity, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return fileIdentity{}, err
	}
	return fileIdentity{info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow}, nil
}

// Sem pedir leitura, só os atributos: abrir assim não conta para o compartilhamento e não
// atrapalha o Discord de renomear o arquivo nesse instante.
func pathIdentity(path string) (fileIdentity, error) {
	handle, err := openShared(path, 0)
	if err != nil {
		return fileIdentity{}, err
	}
	defer windows.CloseHandle(handle)
	return handleIdentity(handle)
}

func (t *logTail) open(now time.Time) bool {
	if now.Before(t.nextTry) {
		return false
	}
	t.nextTry = now.Add(logRetryInterval)
	handle, err := openShared(t.path, windows.GENERIC_READ)
	if err != nil {
		return false
	}
	id, err := handleIdentity(handle)
	if err != nil {
		windows.CloseHandle(handle)
		return false
	}
	file := os.NewFile(uintptr(handle), t.path)
	// O que já estava no arquivo quando o serviço subiu é de antes: começa do fim. Um
	// arquivo novo, depois da troca, é lido desde o começo. E o mesmo arquivo de antes,
	// que só sumiu por um instante, continua de onde parou, sem repetir transmissões velhas.
	var seekErr error
	switch {
	case !t.opened:
		_, seekErr = file.Seek(0, io.SeekEnd)
	case id == t.id:
		_, seekErr = file.Seek(t.lastOffset, io.SeekStart)
	}
	if seekErr != nil {
		file.Close()
		return false
	}
	t.file, t.id, t.opened = file, id, true
	t.buf.reset()
	t.nextCheck = now.Add(logRotateCheck)
	return true
}

func (t *logTail) drain(line func(string)) {
	for {
		n, err := t.file.Read(t.chunk)
		if n > 0 {
			t.buf.feed(t.chunk[:n], line)
		}
		if err != nil || n == 0 {
			return
		}
	}
}

func (t *logTail) poll(now time.Time, line func(string)) {
	if t.file == nil && !t.open(now) {
		return
	}
	t.drain(line)
	if now.Before(t.nextCheck) {
		return
	}
	t.nextCheck = now.Add(logRotateCheck)

	// Arquivo que encolheu foi zerado por quem escreve: volta para o começo.
	if info, err := t.file.Stat(); err == nil {
		if pos, err := t.file.Seek(0, io.SeekCurrent); err == nil && info.Size() < pos {
			t.file.Seek(0, io.SeekStart)
			t.buf.reset()
		}
	}
	id, err := pathIdentity(t.path)
	if err == nil && id == t.id {
		return
	}
	// Outro arquivo no lugar, ou nenhum ainda: o resto do antigo já foi lido acima.
	t.lastOffset, _ = t.file.Seek(0, io.SeekCurrent)
	t.file.Close()
	t.file = nil
	t.nextTry = time.Time{}
	if err == nil && t.open(now) {
		t.drain(line)
	}
}

func (t *logTail) close() {
	if t.file != nil {
		t.file.Close()
		t.file = nil
	}
}

// profileDir acha a pasta do usuário dono do serviço. O serviço roda como sistema, e o
// %APPDATA% dele não é o da pessoa que usa o Discord.
func profileDir(sid string) (string, error) {
	if sid == "" {
		return "", errors.New("serviço sem dono registrado")
	}
	key, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList\`+sid, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer key.Close()
	path, _, err := key.GetStringValue("ProfileImagePath")
	if err != nil {
		return "", err
	}
	return registry.ExpandString(path)
}

// discordLogPaths são os renderer_js.log de cada canal do Discord do dono do serviço. Os
// que não existem ficam na lista mesmo: o Discord pode ser instalado depois.
func discordLogPaths(sid string) []string {
	profile, err := profileDir(sid)
	if err != nil {
		return nil
	}
	var paths []string
	for _, key := range channelOrder {
		folder := strings.ToLower(channels[key].Folder)
		paths = append(paths, filepath.Join(profile, "AppData", "Roaming", folder, "logs", "renderer_js.log"))
	}
	return paths
}

func discordLogSources(paths []string) []lineSource {
	var sources []lineSource
	for _, path := range paths {
		sources = append(sources, newLogTail(path))
	}
	return sources
}
