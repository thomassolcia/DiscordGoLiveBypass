package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestTunnelStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := tunnelState{PID: 42, Phase: phaseReady, Address: "10.2.0.2/32", Endpoint: "203.0.113.10:51820", Routes: 5, ConfHash: "abc"}
	if err := writeTunnelState(dir, want); err != nil {
		t.Fatal(err)
	}

	got, ok := readTunnelState(dir)
	if !ok {
		t.Fatal("readTunnelState nao leu o que acabou de ser gravado")
	}
	got.At = 0
	if got != want {
		t.Errorf("readTunnelState = %+v, queria %+v", got, want)
	}

	// Nenhum temporário pode sobrar na pasta.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("sobraram arquivos na pasta: %v", entries)
	}
}

func TestConfHashMudaComOArquivo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wireguard.conf")
	os.WriteFile(path, []byte(sampleConf), 0o600)
	first, err := confHash(path)
	if err != nil {
		t.Fatal(err)
	}

	os.WriteFile(path, []byte(sampleConf+"\n# outro servidor\n"), 0o600)
	second, _ := confHash(path)
	if first == second || len(first) != 16 {
		t.Errorf("confHash = %q e %q, queria dois valores diferentes de 16 caracteres", first, second)
	}
}

// O próprio processo do teste faz o papel do túnel: está vivo e grava o estado com o PID
// dele. Um estado de outro PID é sobra de execução anterior e não pode ser aceito.
func TestWaitTunnelReady(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("processAlive so existe no Windows")
	}
	dir := t.TempDir()
	self := os.Getpid()

	writeTunnelState(dir, tunnelState{PID: self + 1, Phase: phaseReady})
	go func() {
		time.Sleep(300 * time.Millisecond)
		writeTunnelState(dir, tunnelState{PID: self, Phase: phaseReady, Routes: 7})
	}()

	state, err := waitTunnelReady(dir, self, 5*time.Second)
	if err != nil || state.Routes != 7 {
		t.Fatalf("waitTunnelReady = %+v, %v", state, err)
	}

	writeTunnelState(dir, tunnelState{PID: self, Phase: phaseFailed, Error: "driver recusou"})
	if _, err := waitTunnelReady(dir, self, 5*time.Second); err == nil || err.Error() != "driver recusou" {
		t.Errorf("waitTunnelReady com erro = %v, queria a mensagem do tunel", err)
	}
}

func TestHoldActive(t *testing.T) {
	dir := t.TempDir()
	if holdActive(dir) {
		t.Fatal("holdActive sem arquivo deveria ser falso")
	}
	touchHold(dir)
	if !holdActive(dir) {
		t.Fatal("holdActive logo depois do touchHold deveria ser verdadeiro")
	}
	old := time.Now().Add(-2 * holdWindow)
	os.Chtimes(holdPath(dir), old, old)
	if holdActive(dir) {
		t.Error("holdActive com marca vencida deveria ser falso")
	}
}
