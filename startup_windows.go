//go:build windows

package main

import (
	"errors"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Quem inicia com o Windows é a janela, na bandeja, e não só o serviço: um túnel ligado sem
// nenhum sinal na tela é justamente o que a pessoa não deveria ter. A entrada fica no
// registro da pessoa, que é quem vê o ícone e o único que a janela, sem administrador, mexe.
const (
	runValueName = "DiscordGoLiveBypass"
	trayFlag     = "-bandeja"
)

type startupEntry struct {
	runKey      string
	approvedKey string
}

var userStartup = startupEntry{
	runKey:      `Software\Microsoft\Windows\CurrentVersion\Run`,
	approvedKey: `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`,
}

func startupCommand(exe string) string { return `"` + exe + `" ` + trayFlag }

// O Gerenciador de Tarefas desliga um item do Iniciar sem apagá-lo: grava em
// StartupApproved um valor com o primeiro byte ímpar. Par, ou nenhum valor, é ligado.
func approvedEnabled(data []byte) bool { return len(data) == 0 || data[0]&1 == 0 }

// state diz se a entrada existe e se a pessoa a desligou por fora, no Gerenciador de Tarefas
// ou nas Configurações do Windows.
func (e startupEntry) state() (exists, approved bool) {
	key, err := registry.OpenKey(registry.CURRENT_USER, e.runKey, registry.QUERY_VALUE)
	if err != nil {
		return false, true
	}
	_, _, err = key.GetStringValue(runValueName)
	key.Close()
	if err != nil {
		return false, true
	}

	key, err = registry.OpenKey(registry.CURRENT_USER, e.approvedKey, registry.QUERY_VALUE)
	if err != nil {
		return true, true
	}
	defer key.Close()
	data, _, err := key.GetBinaryValue(runValueName)
	if err != nil {
		return true, true
	}
	return true, approvedEnabled(data)
}

// set grava ou apaga a entrada. Ligar também apaga a marca de desligado do Gerenciador de
// Tarefas, senão a escolha feita aqui não valeria.
func (e startupEntry) set(on bool, exe string) error {
	if !on {
		if err := deleteValue(e.runKey, runValueName); err != nil {
			return err
		}
		return deleteValue(e.approvedKey, runValueName)
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, e.runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	err = key.SetStringValue(runValueName, startupCommand(exe))
	key.Close()
	if err != nil {
		return err
	}
	return deleteValue(e.approvedKey, runValueName)
}

func deleteValue(path, name string) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return nil
	}
	if err != nil {
		return err
	}
	defer key.Close()
	if err := key.DeleteValue(name); err != nil && !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return err
	}
	return nil
}

// wantStartup junta as duas metades da escolha, o tipo de início do serviço e a entrada
// do Iniciar. O serviço manda, porque é o que a janela muda; a exceção é a pessoa ter
// desligado a entrada por fora, e aí o serviço acompanha, para o túnel não subir sem ícone.
// Entrada que não existe com o serviço automático é a de quem veio da 2.2 ou acabou de
// instalar, e passa a existir.
func wantStartup(serviceAutomatic, entryExists, entryApproved bool) bool {
	return serviceAutomatic && (!entryExists || entryApproved)
}
