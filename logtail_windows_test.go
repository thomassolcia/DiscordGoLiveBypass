//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func escrever(t *testing.T, path, texto string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString(texto); err != nil {
		t.Fatal(err)
	}
}

func lerLinhas(tail *logTail, agora time.Time) []string {
	var linhas []string
	tail.poll(agora, func(linha string) { linhas = append(linhas, linha) })
	return linhas
}

// O Discord renomeia o renderer_js.log para renderer_js.old.log perto dos 10 MB. Com o
// serviço lendo, essa troca precisa continuar funcionando, e o arquivo novo precisa ser
// lido do começo.
func TestLogTailAcompanhaATrocaDeArquivo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "renderer_js.log")
	escrever(t, path, "antes do servico\n")

	tail := newLogTail(path)
	defer tail.close()
	agora := time.Now()
	if linhas := lerLinhas(tail, agora); len(linhas) != 0 {
		t.Fatalf("leu o que ja estava no arquivo: %q", linhas)
	}

	escrever(t, path, "primeira\nsegu")
	agora = agora.Add(logPollInterval)
	if linhas := lerLinhas(tail, agora); strings.Join(linhas, "|") != "primeira" {
		t.Fatalf("linhas = %q, queria so a primeira inteira", linhas)
	}
	escrever(t, path, "nda\nultima do antigo\n")

	if err := os.Rename(path, filepath.Join(dir, "renderer_js.old.log")); err != nil {
		t.Fatalf("o Discord nao conseguiria trocar o arquivo com o servico lendo: %v", err)
	}
	escrever(t, path, "do novo\n")

	agora = agora.Add(logRotateCheck)
	if linhas := lerLinhas(tail, agora); strings.Join(linhas, "|") != "segunda|ultima do antigo|do novo" {
		t.Fatalf("linhas = %q, queria o resto do antigo e o comeco do novo", linhas)
	}
	escrever(t, path, "depois\n")
	if linhas := lerLinhas(tail, agora.Add(logPollInterval)); strings.Join(linhas, "|") != "depois" {
		t.Errorf("linhas = %q, queria a linha nova do arquivo novo", linhas)
	}
}

func TestLogTailEsperaOArquivoAparecer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "renderer_js.log")
	tail := newLogTail(path)
	defer tail.close()

	agora := time.Now()
	if linhas := lerLinhas(tail, agora); len(linhas) != 0 {
		t.Fatalf("linhas = %q sem arquivo", linhas)
	}
	escrever(t, path, "velha\n")
	agora = agora.Add(logRetryInterval)
	lerLinhas(tail, agora)
	escrever(t, path, "nova\n")
	if linhas := lerLinhas(tail, agora.Add(logPollInterval)); strings.Join(linhas, "|") != "nova" {
		t.Errorf("linhas = %q, queria so a escrita depois de abrir", linhas)
	}
}

func TestLogTailVoltaAoComecoDoArquivoZerado(t *testing.T) {
	path := filepath.Join(t.TempDir(), "renderer_js.log")
	escrever(t, path, "bem comprida para o arquivo encolher depois\n")
	tail := newLogTail(path)
	defer tail.close()
	agora := time.Now()
	lerLinhas(tail, agora)

	if err := os.Truncate(path, 0); err != nil {
		t.Fatal(err)
	}
	escrever(t, path, "zerado\n")
	agora = agora.Add(logRotateCheck)
	lerLinhas(tail, agora)
	if linhas := lerLinhas(tail, agora.Add(logPollInterval)); strings.Join(linhas, "|") != "zerado" {
		t.Errorf("linhas = %q, queria a linha escrita depois de zerar", linhas)
	}
}

func TestDiscordLogPathsNaPastaDoUsuario(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("sem pasta do usuario neste ambiente")
	}
	paths := discordLogPaths(currentUserSID())
	if len(paths) != len(channelOrder) {
		t.Fatalf("discordLogPaths = %q, queria um por canal", paths)
	}
	quer := filepath.Join(home, "AppData", "Roaming", "discord", "logs", "renderer_js.log")
	if !strings.EqualFold(paths[0], quer) {
		t.Errorf("caminho do Discord = %s, queria %s", paths[0], quer)
	}
	if !strings.HasSuffix(paths[2], filepath.Join("discordcanary", "logs", "renderer_js.log")) {
		t.Errorf("caminho do Canary = %s", paths[2])
	}
	if discordLogPaths("") != nil {
		t.Error("servico sem dono devolveu caminhos")
	}
}
