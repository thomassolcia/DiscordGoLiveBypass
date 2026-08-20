package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Preenchida no build pelo -ldflags. Quem compila na mao fica em "dev" e nunca e avisado
// de versao nova: comparar um binario local com a ultima release so daria alarme falso.
var version = "dev"

const (
	releasesAPI   = "https://api.github.com/repos/thomassolcia/DiscordGoLiveBypass/releases/latest"
	releasesPage  = "https://github.com/thomassolcia/DiscordGoLiveBypass/releases/latest"
	updateTimeout = 4 * time.Second
	updateGrace   = time.Second
	maxUpdateBody = 64 * 1024
)

// A consulta sai numa goroutine no comeco da execucao e o resultado e lido no fim, quando
// ja terminou faz tempo. Ninguem espera por ela para abrir o Discord.
func CheckUpdate(enabled bool) <-chan string {
	newer := make(chan string, 1)

	current := parseVersion(version)
	if !enabled || current == nil {
		close(newer)
		return newer
	}

	go func() {
		defer close(newer)

		tag, ok := latestRelease()
		if !ok {
			return
		}
		if newerThan(parseVersion(tag), current) {
			newer <- tag
		}
	}()
	return newer
}

// Falha em silencio de proposito: sem rede, com a API fora do ar ou no limite de requisicoes,
// o programa nao tem nada de util a dizer e o assunto dele e outro.
func latestRelease() (string, bool) {
	client := &http.Client{Timeout: updateTimeout}

	req, err := http.NewRequest(http.MethodGet, releasesAPI, nil)
	if err != nil {
		return "", false
	}
	// A API do GitHub recusa requisicao sem User-Agent.
	req.Header.Set("User-Agent", "DiscordGoLiveBypass/"+version)
	req.Header.Set("Accept", "application/vnd.github+json")

	res, err := client.Do(req)
	if err != nil {
		return "", false
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return "", false
	}

	var payload struct {
		Tag string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, maxUpdateBody)).Decode(&payload); err != nil {
		return "", false
	}
	return payload.Tag, true
}

// Mesma comparacao numerica que acha a pasta app-<versao> mais nova do Discord. Sufixo de
// pre-lancamento nao vira numero, entao uma v2.0.0-beta simplesmente nao e anunciada.
func parseVersion(raw string) []int {
	var parts []int
	for _, piece := range strings.Split(strings.TrimPrefix(strings.TrimSpace(raw), "v"), ".") {
		number, err := strconv.Atoi(piece)
		if err != nil {
			return nil
		}
		parts = append(parts, number)
	}
	return parts
}

func announceUpdate(ui *UI, newer <-chan string) {
	select {
	case tag, ok := <-newer:
		if !ok || tag == "" {
			return
		}
		ui.Warn("tem uma versao mais nova: %s, esta rodando a %s", tag, version)
		ui.Detail("%s", releasesPage)
	case <-time.After(updateGrace):
		// Consulta lenta demais. Avisar depois que a janela fechou nao serve para nada.
	}
}
