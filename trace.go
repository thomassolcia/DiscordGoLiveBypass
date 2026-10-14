package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// O trace que a Cloudflare responde no gateway diz o IP e o país de quem pergunta, e é o
// IP do gateway que decide o que a conta pode fazer. No discord.com esse caminho dá 403.
// O location-metadata é a mesma pergunta feita ao próprio Discord, mas ele limita por IP
// em poucos pedidos, e o servidor gratuito da VPN é dividido com muita gente: fica de
// reserva. Os dois endereços ficam dentro do túnel, então a pergunta faz o caminho do app.
const (
	traceURL    = "https://gateway.discord.gg/cdn-cgi/trace"
	locationURL = "https://discord.com/api/v9/auth/location-metadata"
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
	trace, traceErr := fetch(client, traceURL)
	if traceErr == nil {
		if ip, country := parseTrace(trace); country != "" {
			return exitInfo{IP: ip, Country: country, Latency: time.Since(started)}, nil
		}
		traceErr = fmt.Errorf("o gateway nao informou o pais de saida")
	}

	started = time.Now()
	body, err := fetch(client, locationURL)
	if err != nil {
		return exitInfo{}, fmt.Errorf("%s; e o location-metadata: %s", DescribeNetwork(traceErr), DescribeNetwork(err))
	}
	country := parseLocation(body)
	if country == "" {
		return exitInfo{}, fmt.Errorf("o Discord nao informou o pais de saida")
	}
	return exitInfo{IP: "nao informado", Country: country, Latency: time.Since(started)}, nil
}

// O código ISO sozinho não diz nada para quem só quer saber se saiu do Brasil.
var countryNames = map[string]string{
	"AR": "Argentina", "AT": "Áustria", "AU": "Austrália", "BE": "Bélgica",
	"BG": "Bulgária", "BR": "Brasil", "CA": "Canadá", "CH": "Suíça",
	"CL": "Chile", "CO": "Colômbia", "CZ": "Tchéquia", "DE": "Alemanha",
	"DK": "Dinamarca", "EE": "Estônia", "ES": "Espanha", "FI": "Finlândia",
	"FR": "França", "GB": "Reino Unido", "GR": "Grécia", "HK": "Hong Kong",
	"HU": "Hungria", "IE": "Irlanda", "IL": "Israel", "IN": "Índia",
	"IS": "Islândia", "IT": "Itália", "JP": "Japão", "KR": "Coreia do Sul",
	"LT": "Lituânia", "LU": "Luxemburgo", "LV": "Letônia", "MD": "Moldávia",
	"MX": "México", "MY": "Malásia", "NL": "Países Baixos", "NO": "Noruega",
	"NZ": "Nova Zelândia", "PE": "Peru", "PL": "Polônia", "PT": "Portugal",
	"PY": "Paraguai", "RO": "Romênia", "RS": "Sérvia", "SE": "Suécia",
	"SG": "Singapura", "SK": "Eslováquia", "TR": "Turquia", "TW": "Taiwan",
	"UA": "Ucrânia", "US": "Estados Unidos", "UY": "Uruguai", "ZA": "África do Sul",
}

func CountryLabel(code string) string {
	code = strings.ToUpper(code)
	if name, ok := countryNames[code]; ok {
		return fmt.Sprintf("%s (%s)", name, code)
	}
	return code
}
