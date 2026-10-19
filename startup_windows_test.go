//go:build windows

package main

import (
	"fmt"
	"os"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// Chaves de teste no registro de quem roda os testes, longe do Iniciar de verdade.
func entradaDeTeste(t *testing.T) startupEntry {
	t.Helper()
	base := fmt.Sprintf(`Software\DiscordGoLiveBypass-teste-%d`, os.Getpid())
	e := startupEntry{runKey: base + `\Run`, approvedKey: base + `\Approved`}
	t.Cleanup(func() {
		registry.DeleteKey(registry.CURRENT_USER, e.runKey)
		registry.DeleteKey(registry.CURRENT_USER, e.approvedKey)
		registry.DeleteKey(registry.CURRENT_USER, base)
	})
	return e
}

func desligarNoGerenciador(t *testing.T, e startupEntry, primeiro byte) {
	t.Helper()
	key, _, err := registry.CreateKey(registry.CURRENT_USER, e.approvedKey, registry.SET_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	defer key.Close()
	if err := key.SetBinaryValue(runValueName, []byte{primeiro, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
}

func TestStartupCommandAbreNaBandeja(t *testing.T) {
	got := startupCommand(`C:\Program Files\DiscordGoLiveBypass\DiscordGoLiveBypass.exe`)
	want := `"C:\Program Files\DiscordGoLiveBypass\DiscordGoLiveBypass.exe" -bandeja`
	if got != want {
		t.Errorf("startupCommand = %s, queria %s", got, want)
	}
}

func TestApprovedEnabledLeOByteDoGerenciador(t *testing.T) {
	for _, c := range []struct {
		data []byte
		quer bool
	}{
		{nil, true},
		{[]byte{0x02, 0, 0}, true},
		{[]byte{0x06, 0, 0}, true},
		{[]byte{0x03, 0xa1, 0x7f}, false},
		{[]byte{0x07}, false},
	} {
		if got := approvedEnabled(c.data); got != c.quer {
			t.Errorf("approvedEnabled(% x) = %v, queria %v", c.data, got, c.quer)
		}
	}
}

func TestStartupEntryLigaEDesliga(t *testing.T) {
	e := entradaDeTeste(t)
	if exists, approved := e.state(); exists || !approved {
		t.Fatalf("sem nada gravado: existe %v, liberada %v", exists, approved)
	}
	// Desligar o que não existe não é erro: é o caso de quem nunca ligou.
	if err := e.set(false, `C:\x.exe`); err != nil {
		t.Fatalf("desligar sem entrada: %v", err)
	}

	if err := e.set(true, `C:\Arquivos de Programas\x.exe`); err != nil {
		t.Fatal(err)
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, e.runKey, registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	valor, _, err := key.GetStringValue(runValueName)
	key.Close()
	if err != nil || valor != `"C:\Arquivos de Programas\x.exe" -bandeja` {
		t.Errorf("valor gravado = %q, %v", valor, err)
	}
	if exists, approved := e.state(); !exists || !approved {
		t.Errorf("depois de ligar: existe %v, liberada %v", exists, approved)
	}

	if err := e.set(false, ""); err != nil {
		t.Fatal(err)
	}
	if exists, _ := e.state(); exists {
		t.Error("a entrada continuou lá depois de desligar")
	}
}

// Desligada no Gerenciador de Tarefas, a entrada continua existindo; ligar pela janela
// precisa tirar a marca, senão o Windows continuaria sem abrir o programa.
func TestStartupEntryRespeitaEDesfazOGerenciador(t *testing.T) {
	e := entradaDeTeste(t)
	if err := e.set(true, `C:\x.exe`); err != nil {
		t.Fatal(err)
	}
	desligarNoGerenciador(t, e, 0x03)
	if exists, approved := e.state(); !exists || approved {
		t.Fatalf("desligada no gerenciador: existe %v, liberada %v", exists, approved)
	}

	if err := e.set(true, `C:\x.exe`); err != nil {
		t.Fatal(err)
	}
	if _, approved := e.state(); !approved {
		t.Error("ligar pela janela não tirou a marca do Gerenciador de Tarefas")
	}

	desligarNoGerenciador(t, e, 0x03)
	if err := e.set(false, ""); err != nil {
		t.Fatal(err)
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, e.approvedKey, registry.QUERY_VALUE)
	if err == nil {
		_, _, err = key.GetBinaryValue(runValueName)
		key.Close()
	}
	if err == nil {
		t.Error("desligar deixou a marca do Gerenciador para trás")
	}
}

func TestWantStartup(t *testing.T) {
	for _, c := range []struct {
		automatico, existe, liberada, quer bool
		caso                               string
	}{
		{true, false, true, true, "veio da 2.2 ou acabou de instalar"},
		{true, true, true, true, "tudo ligado"},
		{true, true, false, false, "desligada no Gerenciador de Tarefas"},
		{false, true, true, false, "serviço manual com entrada sobrando"},
		{false, false, true, false, "tudo desligado"},
	} {
		if got := wantStartup(c.automatico, c.existe, c.liberada); got != c.quer {
			t.Errorf("%s: wantStartup = %v, queria %v", c.caso, got, c.quer)
		}
	}
}
