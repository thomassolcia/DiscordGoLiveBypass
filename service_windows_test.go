//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Instalação da 2.1 não tem modo no settings.json e passa a dividir; o modo escolhido fica
// guardado junto com o resto, sem mexer no dono nem no túnel ligado.
func TestSetModeGuardaSoModoConhecido(t *testing.T) {
	dir := t.TempDir()
	antigo := `{"enabled": false, "owner_sid": "S-1-5-21-1-2-3-1001"}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(antigo), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &tunnelService{dir: dir, engine: NewEngine(), settings: loadSettings(dir)}

	if resp := s.handle(ipcRequest{Cmd: cmdStatus}); resp.Status.Mode != modeStream {
		t.Errorf("modo sem nada guardado = %q, queria %q", resp.Status.Mode, modeStream)
	}

	resp := s.handle(ipcRequest{Cmd: cmdSetMode, Mode: "turbo"})
	if resp.OK || resp.Error == "" {
		t.Errorf("modo desconhecido aceito: %+v", resp)
	}
	if loadSettings(dir).Mode != "" {
		t.Error("modo desconhecido foi guardado")
	}

	// Duas trocas seguidas com o túnel desligado não podem travar o serviço.
	s.handle(ipcRequest{Cmd: cmdSetMode, Mode: modeStream})
	resp = s.handle(ipcRequest{Cmd: cmdSetMode, Mode: modeFull})
	if !resp.OK || resp.Status.Mode != modeFull {
		t.Errorf("trocar para completo: %+v", resp)
	}
	guardado := loadSettings(dir)
	if guardado.Mode != modeFull || guardado.OwnerSID != "S-1-5-21-1-2-3-1001" || guardado.Enabled {
		t.Errorf("settings depois da troca = %+v", guardado)
	}
	if s.engine.currentMode() != modeFull {
		t.Errorf("motor ficou no modo %q", s.engine.currentMode())
	}
}
