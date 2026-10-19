//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
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

type fakeStart struct {
	on      bool
	err     error
	setErr  error
	chamado int
}

func (f *fakeStart) automatic() (bool, error) { return f.on, f.err }

func (f *fakeStart) setAutomatic(on bool) error {
	f.chamado++
	if f.setErr != nil {
		return f.setErr
	}
	f.on = on
	return nil
}

func TestSetAutostartMudaOTipoDeInicio(t *testing.T) {
	inicio := &fakeStart{on: true}
	s := &tunnelService{dir: t.TempDir(), engine: NewEngine(), startup: inicio}

	resp := s.handle(ipcRequest{Cmd: cmdStatus})
	if resp.Status.Autostart == nil || !*resp.Status.Autostart {
		t.Fatalf("status com serviço automático = %v, queria true", resp.Status.Autostart)
	}

	resp = s.handle(ipcRequest{Cmd: cmdSetAutostart, Autostart: false})
	if !resp.OK || inicio.on || resp.Status.Autostart == nil || *resp.Status.Autostart {
		t.Errorf("desligar: resp %+v, serviço automático %v", resp, inicio.on)
	}
	resp = s.handle(ipcRequest{Cmd: cmdSetAutostart, Autostart: true})
	if !resp.OK || !inicio.on {
		t.Errorf("ligar de novo: resp %+v, serviço automático %v", resp, inicio.on)
	}
}

func TestSetAutostartDevolveOErroDoWindows(t *testing.T) {
	inicio := &fakeStart{on: true, setErr: errors.New("acesso negado")}
	s := &tunnelService{dir: t.TempDir(), engine: NewEngine(), startup: inicio}

	resp := s.handle(ipcRequest{Cmd: cmdSetAutostart, Autostart: false})
	if resp.OK || !strings.Contains(resp.Error, "acesso negado") {
		t.Errorf("falha do Windows virou %+v", resp)
	}
	if resp.Status.Autostart == nil || !*resp.Status.Autostart {
		t.Error("o status precisa continuar dizendo o que o Windows tem de fato")
	}
}

// Sem conseguir ler o tipo de início, o status não inventa um valor: a janela trataria um
// "falso" como escolha da pessoa e apagaria a entrada do Iniciar.
func TestStatusSemTipoDeInicioNaoChuta(t *testing.T) {
	s := &tunnelService{dir: t.TempDir(), engine: NewEngine(), startup: &fakeStart{err: errors.New("sem SCM")}}
	if got := s.handle(ipcRequest{Cmd: cmdStatus}).Status.Autostart; got != nil {
		t.Errorf("autostart sem leitura = %v, queria ausente", *got)
	}
	s.startup = nil
	if got := s.handle(ipcRequest{Cmd: cmdStatus}).Status.Autostart; got != nil {
		t.Errorf("autostart sem controle = %v, queria ausente", *got)
	}
	if resp := s.handle(ipcRequest{Cmd: cmdSetAutostart, Autostart: true}); resp.OK {
		t.Error("set_autostart sem controle foi aceito")
	}
}

func TestInstalarPreservaQuemNaoIniciaComOWindows(t *testing.T) {
	casos := []struct {
		existe bool
		atual  uint32
		quer   uint32
	}{
		{false, 0, mgr.StartAutomatic},
		{true, mgr.StartAutomatic, mgr.StartAutomatic},
		{true, mgr.StartManual, mgr.StartManual},
		{true, mgr.StartDisabled, mgr.StartAutomatic},
	}
	for _, c := range casos {
		if got := installStartType(c.existe, c.atual); got != c.quer {
			t.Errorf("installStartType(%v, %d) = %d, queria %d", c.existe, c.atual, got, c.quer)
		}
	}
}

// O dono ganha consultar e iniciar (RP), nunca parar (WP) nem reconfigurar (DC).
func TestServiceSDDLDeixaODonoSoIniciar(t *testing.T) {
	const dono = "S-1-5-21-1-2-3-1001"
	sddl := serviceSDDL(dono)
	if _, err := windows.SecurityDescriptorFromString(sddl); err != nil {
		t.Fatalf("SDDL inválido: %v", err)
	}
	ace := "(A;;CCLCSWRPLOCRRC;;;" + dono + ")"
	if !strings.Contains(sddl, ace) {
		t.Fatalf("SDDL sem a entrada do dono: %s", sddl)
	}
	if strings.Contains(ace, "WP") || strings.Contains(ace, "DC") {
		t.Errorf("o dono ganhou mais do que iniciar: %s", ace)
	}
	for _, invalido := range []string{"", "lixo", "S-1-5-21-"} {
		if strings.Count(serviceSDDL(invalido), "(A;") != 4 {
			t.Errorf("serviceSDDL(%q) acrescentou uma entrada", invalido)
		}
	}
}
