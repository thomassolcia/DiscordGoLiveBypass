package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	freeProxyAPI = "https://api.proxyscrape.com/v4/free-proxy-list/get?request=display_proxies&protocol=socks5&proxy_format=protocolipport&format=json&timeout=1500"
	discordHost  = "discord.com"
	geoHost      = "ifconfig.co"

	maxListBytes     = 1024 * 1024
	probeTimeout     = 6 * time.Second
	parallelProbes   = 10
	maxCandidates    = 40
	minUptime        = 90.0
	maxListedTimeout = 1500.0
	cacheMaxAge      = 24 * time.Hour
	fastProbeTimeout = 2500 * time.Millisecond
	torPortTimeout   = 400 * time.Millisecond
)

// Bridge meek, Tor Browser, daemon e Brave, nessa ordem.
var torPorts = []int{9052, 9150, 9050, 9250}

// Essa porta intercepta em vez de encaminhar: responde bonito e entrega conteúdo adulterado.
var interceptingPorts = map[int]bool{4145: true}

type listEntry struct {
	Proxy   string  `json:"proxy"`
	Alive   bool    `json:"alive"`
	Uptime  float64 `json:"uptime"`
	Timeout float64 `json:"timeout"`
	IPData  struct {
		CountryCode string `json:"countryCode"`
	} `json:"ip_data"`
}

func ParseExcluded(raw string) map[string]bool {
	excluded := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		code := strings.ToUpper(strings.TrimSpace(part))
		if len(code) != 2 {
			continue
		}
		valid := true
		for _, c := range code {
			if c < 'A' || c > 'Z' {
				valid = false
			}
		}
		if valid {
			excluded[code] = true
		}
	}
	return excluded
}

func downloadText(url string) (string, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	res, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("a lista respondeu HTTP %d", res.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(res.Body, maxListBytes))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// O país que vem na lista é só uma dica para descartar os óbvios. Quem decide de verdade é
// o ExitCountry, mais adiante.
func rankFreeProxies(body string, excluded map[string]bool) ([]Endpoint, error) {
	var payload struct {
		Proxies []listEntry `json:"proxies"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return nil, err
	}

	type scored struct {
		endpoint Endpoint
		uptime   float64
		timeout  float64
	}

	var usable []scored
	for _, entry := range payload.Proxies {
		if !entry.Alive {
			continue
		}

		endpoint, ok := ParseProxy(entry.Proxy)
		if !ok || interceptingPorts[endpoint.Port] {
			continue
		}

		listed := entry.Timeout
		if listed == 0 {
			listed = maxListedTimeout
		}
		if entry.Uptime < minUptime || listed > maxListedTimeout {
			continue
		}

		if excluded[strings.ToUpper(entry.IPData.CountryCode)] {
			continue
		}

		usable = append(usable, scored{endpoint, entry.Uptime, listed})
	}

	sort.SliceStable(usable, func(i, j int) bool {
		if usable[i].uptime != usable[j].uptime {
			return usable[i].uptime > usable[j].uptime
		}
		return usable[i].timeout < usable[j].timeout
	})

	if len(usable) > maxCandidates {
		usable = usable[:maxCandidates]
	}

	ranked := make([]Endpoint, 0, len(usable))
	for _, entry := range usable {
		ranked = append(ranked, entry.endpoint)
	}
	return ranked, nil
}

type probeResult struct {
	endpoint Endpoint
	latency  time.Duration
}

func PickFreeProxy(excluded map[string]bool, deadline time.Time, ui *UI) (Endpoint, time.Duration, error) {
	ui.Busy("baixando a lista publica de proxies")
	body, err := downloadText(freeProxyAPI)
	if err != nil {
		return Endpoint{}, 0, fmt.Errorf("nao consegui baixar a lista de proxies: %w", err)
	}

	candidates, err := rankFreeProxies(body, excluded)
	if err != nil {
		return Endpoint{}, 0, fmt.Errorf("a lista de proxies veio num formato inesperado: %w", err)
	}

	if len(candidates) == 0 {
		return Endpoint{}, 0, errors.New("nenhuma candidata sobreviveu aos filtros da lista")
	}
	ui.Ok("%d candidatas depois dos filtros", len(candidates))

	// Contadores atômicos: a barra é repintada de dentro das goroutines do lote, e a
	// contagem precisa somar o progresso de todos os lotes, não só o atual.
	var tested, alive int64
	ui.Progress(0, len(candidates), "testando conexao com o Discord")

	for start := 0; start < len(candidates); start += parallelProbes {
		if time.Now().After(deadline) {
			return Endpoint{}, 0, errors.New("o prazo acabou antes de achar uma proxy valida")
		}

		end := start + parallelProbes
		if end > len(candidates) {
			end = len(candidates)
		}
		batch := candidates[start:end]

		results := make([]*probeResult, len(batch))
		var wg sync.WaitGroup
		for i, candidate := range batch {
			wg.Add(1)
			go func(i int, candidate Endpoint) {
				defer wg.Done()
				latency, err := Probe(candidate, probeTimeout)
				if err == nil {
					results[i] = &probeResult{candidate, latency}
					atomic.AddInt64(&alive, 1)
				}
				ui.Progress(int(atomic.AddInt64(&tested, 1)), len(candidates),
					"%d responderam ate agora", atomic.LoadInt64(&alive))
			}(i, candidate)
		}
		wg.Wait()

		var working []probeResult
		for _, result := range results {
			if result != nil {
				working = append(working, *result)
			}
		}
		sort.Slice(working, func(i, j int) bool { return working[i].latency < working[j].latency })

		if len(working) == 0 {
			ui.Detail("lote %d: nenhuma das %d respondeu", start/parallelProbes+1, len(batch))
			continue
		}

		for _, result := range working {
			ui.Busy("conferindo o pais de saida de %s", result.endpoint)
			country, err := Accepts(result.endpoint, excluded, probeTimeout)
			if err != nil {
				ui.Detail("%s recusada: %v", result.endpoint, err)
				continue
			}
			ui.Ok("%s serve, %d ms, saida em %s", result.endpoint, result.latency.Milliseconds(), CountryLabel(country))
			return result.endpoint, result.latency, nil
		}
	}

	return Endpoint{}, 0, errors.New("nenhuma proxy da lista passou nos testes")
}

func listening(port int, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func DetectTor(excluded map[string]bool, ui *UI) (Endpoint, time.Duration, bool) {
	for _, port := range torPorts {
		if !listening(port, torPortTimeout) {
			continue
		}

		ui.Busy("porta %d aberta, testando se e um proxy de verdade", port)
		endpoint := Endpoint{Scheme: "socks5", Host: "127.0.0.1", Port: port}
		latency, err := Probe(endpoint, probeTimeout)
		if err != nil {
			ui.Detail("porta %d aberta mas nao respondeu como proxy, ignorando", port)
			continue
		}

		country, err := Accepts(endpoint, excluded, probeTimeout)
		if err != nil {
			ui.Detail("Tor na porta %d recusado: %v", port, err)
			continue
		}

		ui.Ok("Tor local na porta %d, %d ms, saida em %s", port, latency.Milliseconds(), CountryLabel(country))
		return endpoint, latency, true
	}
	return Endpoint{}, 0, false
}

type cacheFile struct {
	Proxy string `json:"proxy"`
	At    int64  `json:"at"`
}

func cachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "DiscordGoLiveBypass", "proxy.json"), nil
}

// Recente não quer dizer viva: quem chama ainda precisa sondar antes de usar.
func ReadCachedProxy() (Endpoint, bool) {
	path, err := cachePath()
	if err != nil {
		return Endpoint{}, false
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return Endpoint{}, false
	}

	var cached cacheFile
	if err := json.Unmarshal(raw, &cached); err != nil {
		return Endpoint{}, false
	}
	if time.Since(time.UnixMilli(cached.At)) > cacheMaxAge {
		return Endpoint{}, false
	}

	return ParseProxy(cached.Proxy)
}

func StoreCachedProxy(e Endpoint) error {
	path, err := cachePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	raw, err := json.Marshal(cacheFile{Proxy: e.String(), At: time.Now().UnixMilli()})
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}
