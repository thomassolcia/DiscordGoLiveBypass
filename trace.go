package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// O país vem do próprio Discord, na mesma resposta que ele usa para decidir o que a conta
// pode fazer. O IP vem do trace que a Cloudflare responde no gateway (no discord.com esse
// caminho dá 403). Os dois endereços ficam dentro do túnel, então a pergunta faz o mesmo
// caminho que o app vai fazer.
const (
	locationURL = "https://discord.com/api/v9/auth/location-metadata"
	traceURL    = "https://gateway.discord.gg/cdn-cgi/trace"
)

type exitInfo struct {
	IP      string
	Country string
	Latency time.Duration
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

func parseTrace(body string) (ip, country string) {
	for _, line := range strings.Split(body, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
		case "ip":
			ip = value
		case "loc":
			country = strings.ToUpper(value)
		}
	}
	return ip, country
}

func parseLocation(body string) string {
	var payload struct {
		Country string `json:"country_code"`
	}
	if json.Unmarshal([]byte(body), &payload) != nil {
		return ""
	}
	return strings.ToUpper(payload.Country)
}

func fetch(client *http.Client, url string) (string, error) {
	res, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, 16*1024))
	if err != nil {
		return "", err
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("o Discord respondeu HTTP %d", res.StatusCode)
	}
	return string(body), nil
}

// Conexão nova a cada chamada: reaproveitar uma aberta antes do túnel subir mediria a rota
// antiga, que é justamente a que não interessa.
func CheckExit(timeout time.Duration) (exitInfo, error) {
	client := &http.Client{
		Timeout:   timeout,
		Transport: &http.Transport{DisableKeepAlives: true, Proxy: nil},
	}

	started := time.Now()
	body, err := fetch(client, locationURL)
	if err != nil {
		return exitInfo{}, err
	}
	country := parseLocation(body)
	if country == "" {
		return exitInfo{}, fmt.Errorf("o Discord nao informou o pais de saida")
	}
	exit := exitInfo{Country: country, Latency: time.Since(started)}

	// O IP é só informativo: sem ele a saída continua conferida pelo país.
	if trace, err := fetch(client, traceURL); err == nil {
		exit.IP, _ = parseTrace(trace)
	}
	if exit.IP == "" {
		exit.IP = "nao informado"
	}
	return exit, nil
}
