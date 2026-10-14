package main

import (
	"os"
	"testing"
	"time"
)

func TestParseExcluded(t *testing.T) {
	got := ParseExcluded(" br , AR, x, 123, ")
	want := map[string]bool{"BR": true, "AR": true}

	if len(got) != len(want) {
		t.Fatalf("ParseExcluded = %v, queria %v", got, want)
	}
	for code := range want {
		if !got[code] {
			t.Errorf("faltou %s em %v", code, got)
		}
	}

	if empty := ParseExcluded(""); len(empty) != 0 {
		t.Errorf("ParseExcluded(\"\") = %v, queria vazio", empty)
	}
}

// Resposta real do location-metadata em outubro de 2026.
func TestParseLocation(t *testing.T) {
	body := `{"consent_required":false,"country_code":"us","promotional_email_opt_in":{"required":true,"pre_checked":true}}`
	if got := parseLocation(body); got != "US" {
		t.Errorf("parseLocation = %q, queria US", got)
	}
	for _, broken := range []string{"", "<html>403</html>", `{"message":"404: Not Found","code":0}`} {
		if got := parseLocation(broken); got != "" {
			t.Errorf("parseLocation(%q) = %q, queria vazio", broken, got)
		}
	}
}

// Resposta real do gateway.discord.gg/cdn-cgi/trace, com o IP trocado por um de documentação.
func TestParseTrace(t *testing.T) {
	body := "fl=123f45\nh=gateway.discord.gg\nip=203.0.113.7\nts=1760400000.123\nvisit_scheme=https\n" +
		"uag=Go-http-client/1.1\ncolo=EWR\nsliver=none\nhttp=http/1.1\nloc=us\ntls=TLSv1.3\nsni=plaintext\nwarp=off\n"

	ip, country := parseTrace(body)
	if ip != "203.0.113.7" || country != "US" {
		t.Errorf("parseTrace = %q, %q, queria 203.0.113.7, US", ip, country)
	}

	if ip, country := parseTrace("<html>bloqueado</html>"); ip != "" || country != "" {
		t.Errorf("parseTrace(html) = %q, %q, queria vazio", ip, country)
	}
}

// Fala com o Discord de verdade, então só roda quando pedido: DGLB_REDE=1 go test -run AoVivo.
// Sem o túnel ligado, o país esperado é o da sua conexão normal.
func TestCheckExitAoVivo(t *testing.T) {
	if os.Getenv("DGLB_REDE") == "" {
		t.Skip("defina DGLB_REDE=1 para consultar o Discord")
	}
	exit, err := CheckExit(10 * time.Second)
	if err != nil {
		t.Fatalf("CheckExit: %v", err)
	}
	t.Logf("o Discord ve %s, ip %s, %d ms", exit.Country, exit.IP, exit.Latency.Milliseconds())
}
