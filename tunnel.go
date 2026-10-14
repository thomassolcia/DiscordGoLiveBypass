package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// O túnel vive num processo separado, elevado e sem janela, porque precisa de
// administrador para criar o adaptador e precisa durar enquanto o Discord estiver aberto.
// O launcher continua sem elevação: assim o Discord abre como o usuário, e não como
// administrador. Os dois conversam por arquivos na pasta de dados.
const (
	adapterName = "DiscordGoLiveBypass"

	phaseStarting = "subindo"
	phaseReady    = "pronto"
	phaseFailed   = "erro"
	phaseStopped  = "encerrado"

	// Tempo que o Discord pode ficar fechado antes de o túnel cair. Cobre o reinício do
	// próprio updater e o -force, que fecha e abre de novo.
	discordAbsenceGrace = 30 * time.Second
	// Quanto o túnel espera o Discord aparecer pela primeira vez antes de desistir.
	discordStartGrace = 3 * time.Minute
	// Um launcher que reaproveita o túnel marca presença por esse tempo, para ele não cair
	// no intervalo entre o Discord antigo fechar e o novo abrir.
	holdWindow = time.Minute
)

type tunnelState struct {
	PID      int    `json:"pid"`
	Phase    string `json:"phase"`
	Error    string `json:"error,omitempty"`
	Address  string `json:"address,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	Routes   int    `json:"routes,omitempty"`
	ConfHash string `json:"conf_hash,omitempty"`
	At       int64  `json:"at"`
}

func statePath(dir string) string { return filepath.Join(dir, "tunnel.json") }
func stopPath(dir string) string  { return filepath.Join(dir, "tunnel.stop") }
func holdPath(dir string) string  { return filepath.Join(dir, "tunnel.hold") }
func logPath(dir string) string   { return filepath.Join(dir, "tunnel.log") }

func readTunnelState(dir string) (tunnelState, bool) {
	raw, err := os.ReadFile(statePath(dir))
	if err != nil {
		return tunnelState{}, false
	}
	var state tunnelState
	if err := json.Unmarshal(raw, &state); err != nil {
		return tunnelState{}, false
	}
	return state, true
}

// Escrita atômica: o launcher lê esse arquivo em laço e não pode pegar ele pela metade.
func writeTunnelState(dir string, state tunnelState) error {
	state.At = time.Now().UnixMilli()
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp := statePath(dir) + fmt.Sprintf(".%d.tmp", os.Getpid())
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, statePath(dir))
}

func requestTunnelStop(dir string) error {
	return os.WriteFile(stopPath(dir), []byte("stop"), 0o600)
}

func touchHold(dir string) error {
	return os.WriteFile(holdPath(dir), []byte(time.Now().Format(time.RFC3339)), 0o600)
}

func holdActive(dir string) bool {
	info, err := os.Stat(holdPath(dir))
	return err == nil && time.Since(info.ModTime()) < holdWindow
}

// Identifica o arquivo sem guardar nada dele: o túnel antigo só é reaproveitado se foi
// subido com a mesma configuração.
func confHash(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:8]), nil
}

var (
	errHelperGone      = errors.New("o processo do tunel fechou antes de ficar pronto")
	errElevationDenied = errors.New("permissao de administrador negada")
)

// O que dá para mostrar de um arquivo de VPN sem expor a chave dele.
type confSummary struct {
	Endpoint string
	Address  string
}

// Espera o processo do túnel com o PID informado dizer que está pronto. Um estado de outro
// PID é sobra de uma execução anterior e é ignorado.
func waitTunnelReady(dir string, pid int, timeout time.Duration) (tunnelState, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if state, ok := readTunnelState(dir); ok && state.PID == pid {
			switch state.Phase {
			case phaseReady:
				return state, nil
			case phaseFailed:
				return state, errors.New(state.Error)
			case phaseStopped:
				return state, errHelperGone
			}
		}
		if !processAlive(pid) {
			if state, ok := readTunnelState(dir); ok && state.PID == pid && state.Phase == phaseFailed {
				return state, errors.New(state.Error)
			}
			return tunnelState{}, errHelperGone
		}
		time.Sleep(250 * time.Millisecond)
	}
	return tunnelState{}, errors.New("o tunel nao ficou pronto a tempo")
}

func waitTunnelGone(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return true
		}
		time.Sleep(250 * time.Millisecond)
	}
	return !processAlive(pid)
}
