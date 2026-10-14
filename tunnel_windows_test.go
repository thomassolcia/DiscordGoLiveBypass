//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConf(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "wg.conf")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestValidateConfArquivoDoProton(t *testing.T) {
	summary, err := ValidateConf(writeConf(t, sampleConf))
	if err != nil {
		t.Fatalf("ValidateConf: %v", err)
	}
	if summary.Endpoint != "203.0.113.10:51820" || summary.Address != "10.2.0.2/32" {
		t.Errorf("ValidateConf = %+v", summary)
	}
}

func TestValidateConfRecusa(t *testing.T) {
	cases := map[string]struct {
		text string
		want string
	}{
		"sem peer": {
			strings.Split(sampleConf, "[Peer]")[0],
			"[Peer]",
		},
		"sem endpoint": {
			strings.Replace(sampleConf, "Endpoint = 203.0.113.10:51820\n", "", 1),
			"Endpoint",
		},
		"so ipv6": {
			strings.Replace(sampleConf, "Address = 10.2.0.2/32", "Address = fd00::2/128", 1),
			"IPv4",
		},
		"servidor dentro do discord": {
			strings.Replace(sampleConf, "203.0.113.10", "66.22.200.10", 1),
			"dentro da rede do Discord",
		},
		// Chave com um caractere a menos: o parser oficial repete o valor recusado no erro.
		"chave truncada": {
			strings.Replace(sampleConf, "BwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwc=", "BwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBw=", 1),
			"nao e uma configuracao WireGuard valida",
		},
		"preshared truncada": {
			strings.Replace(sampleConf, "[Peer]\n", "[Peer]\nPresharedKey = BwcHBwcHBwcHBwcHBwcHBwcHBwc=\n", 1),
			"<chave omitida>",
		},
	}

	for name, c := range cases {
		_, err := ValidateConf(writeConf(t, c.text))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: ValidateConf = %v, queria erro falando de %q", name, err, c.want)
		}
		if err != nil && strings.Contains(err.Error(), "BwcHBwcH") {
			t.Errorf("%s: a mensagem de erro vazou a chave privada: %v", name, err)
		}
	}
}
