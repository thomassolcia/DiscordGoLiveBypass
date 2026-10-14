package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

type Channel struct {
	Key      string
	Folder   string
	Binary   string
	Friendly string
}

var channels = map[string]Channel{
	"stable": {"stable", "Discord", "Discord.exe", "Discord"},
	"ptb":    {"ptb", "DiscordPTB", "DiscordPTB.exe", "Discord PTB"},
	"canary": {"canary", "DiscordCanary", "DiscordCanary.exe", "Discord Canary"},
}

var channelOrder = []string{"stable", "ptb", "canary"}

func LookupChannel(key string) (Channel, error) {
	channel, ok := channels[strings.ToLower(strings.TrimSpace(key))]
	if !ok {
		return Channel{}, fmt.Errorf("canal desconhecido %q, use auto, stable, ptb ou canary", key)
	}
	return channel, nil
}

func DetectChannel() (Channel, string, error) {
	var checked []string
	for _, key := range channelOrder {
		channel := channels[key]
		binary, err := LocateDiscord(channel)
		if err == nil {
			return channel, binary, nil
		}
		checked = append(checked, channel.Friendly)
	}

	return Channel{}, "", fmt.Errorf("nao achei nenhum Discord instalado. Procurei por %s dentro de %%LOCALAPPDATA%%. Se o seu esta em outro lugar, aponte com -exe", strings.Join(checked, ", "))
}

func ChannelForBinary(binary string) Channel {
	name := filepath.Base(binary)
	for _, key := range channelOrder {
		if strings.EqualFold(name, channels[key].Binary) {
			return channels[key]
		}
	}
	return Channel{Key: "custom", Binary: name, Friendly: name}
}

// Ordenar as pastas como texto colocaria app-1.0.998 na frente de app-1.0.9184, e o
// launcher abriria uma versão que o updater já aposentou.
func versionOf(name string) []int {
	raw := strings.TrimPrefix(name, "app-")
	var parts []int
	for _, piece := range strings.Split(raw, ".") {
		number, err := strconv.Atoi(piece)
		if err != nil {
			return nil
		}
		parts = append(parts, number)
	}
	return parts
}

func newerThan(a, b []int) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return len(a) > len(b)
}

// Aponta para o Discord.exe dentro de app-<versão>, e não para o Update.exe da raiz: é pelo
// nome desse binário que o túnel acompanha se o Discord continua aberto.
func LocateDiscord(channel Channel) (string, error) {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return "", fmt.Errorf("LOCALAPPDATA nao esta definido, informe o caminho com -exe")
	}

	root := filepath.Join(local, channel.Folder)
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", fmt.Errorf("nao achei a instalacao do %s em %s", channel.Friendly, root)
	}

	var versions [][]int
	byVersion := map[string]string{}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "app-") {
			continue
		}

		binary := filepath.Join(root, entry.Name(), channel.Binary)
		if info, err := os.Stat(binary); err != nil || info.IsDir() {
			continue
		}

		version := versionOf(entry.Name())
		if version == nil {
			continue
		}

		versions = append(versions, version)
		byVersion[fmt.Sprint(version)] = binary
	}

	if len(versions) == 0 {
		return "", fmt.Errorf("nenhum %s instalado em %s", channel.Binary, root)
	}

	sort.Slice(versions, func(i, j int) bool { return newerThan(versions[i], versions[j]) })
	for _, version := range versions {
		binary := byVersion[fmt.Sprint(version)]
		if complete(filepath.Dir(binary)) {
			return binary, nil
		}
	}

	return "", fmt.Errorf("achei %s em %s, mas nenhuma versao esta completa. Isso e update interrompido: abra o %s normalmente uma vez, deixe ele terminar de se atualizar e rode de novo", channel.Binary, root, channel.Friendly)
}

// Update interrompido deixa a pasta da versão nova com o executável, mas sem os arquivos que
// o Chromium precisa para subir. O processo nasce e morre sozinho, então a pasta mais nova
// não serve só por ser a mais nova.
var runtimeFiles = []string{"icudtl.dat", "resources.pak", "locales"}

func complete(dir string) bool {
	for _, name := range runtimeFiles {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			return false
		}
	}
	return true
}

// O Discord é instância única: subir um segundo processo só acorda a janela do primeiro, e as
// conexões que ele já tinha aberto continuam pela rota de antes do túnel.
func IsRunning(channel Channel) (bool, error) {
	if runtime.GOOS != "windows" {
		return false, fmt.Errorf("checagem de processo so implementada no Windows")
	}

	out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq "+channel.Binary, "/NH", "/FO", "CSV").Output()
	if err != nil {
		return false, err
	}
	return strings.Contains(strings.ToLower(string(out)), strings.ToLower(channel.Binary)), nil
}

func Terminate(channel Channel) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("encerrar processo so implementado no Windows")
	}

	out, err := exec.Command("taskkill", "/IM", channel.Binary, "/F", "/T").CombinedOutput()
	if err != nil {
		return fmt.Errorf("taskkill falhou: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func Launch(binary string, args []string) (int, error) {
	cmd := exec.Command(binary, args...)
	cmd.Dir = filepath.Dir(binary)
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return 0, err
	}

	pid := cmd.Process.Pid
	// Solta o handle: o Discord segue vivo depois que o launcher sai.
	if err := cmd.Process.Release(); err != nil {
		return pid, err
	}
	return pid, nil
}
