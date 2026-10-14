package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Formato do arquivo que o painel do Proton entrega, com chaves de mentira.
const sampleConf = `[Interface]
# Key for teste
# Bouncing = 1
# NetShield = 0
PrivateKey = BwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwc=
Address = 10.2.0.2/32
DNS = 10.2.0.1

[Peer]
# US-FREE#1
PublicKey = CAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAg=
AllowedIPs = 0.0.0.0/0, ::/0
Endpoint = 203.0.113.10:51820
`

func TestLooksLikeWireGuard(t *testing.T) {
	cases := map[string]struct {
		text string
		want bool
	}{
		"arquivo do proton":      {sampleConf, true},
		"sem peer":               {"[Interface]\nPrivateKey = x\n", false},
		"sem chave":              {"[Interface]\nAddress = 10.0.0.2/32\n[Peer]\nPublicKey = y\n", false},
		"chave comentada":        {"[Interface]\n# PrivateKey = x\n[Peer]\nPublicKey = y\n", false},
		"chave no lugar errado":  {"[Peer]\nPrivateKey = x\n", false},
		"conf de outro programa": {"[server]\nport=80\n", false},
	}
	for name, c := range cases {
		if got := looksLikeWireGuard(c.text); got != c.want {
			t.Errorf("%s: looksLikeWireGuard = %v, queria %v", name, got, c.want)
		}
	}
}

func TestFindConfCandidates(t *testing.T) {
	dir := t.TempDir()
	write := func(name, text string, age time.Duration) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		when := time.Now().Add(-age)
		if err := os.Chtimes(path, when, when); err != nil {
			t.Fatal(err)
		}
		return path
	}

	old := write("wg-antigo.conf", sampleConf, 48*time.Hour)
	recent := write("US-FREE-1.CONF", sampleConf, time.Hour)
	write("nginx.conf", "[server]\nport=80\n", 0)
	write("wg-mas-txt.txt", sampleConf, 0)

	found := findConfCandidates([]string{dir, "", filepath.Join(dir, "nao-existe"), dir})
	if len(found) != 2 {
		t.Fatalf("findConfCandidates achou %d, queria 2: %v", len(found), found)
	}
	if found[0].Path != recent || found[1].Path != old {
		t.Errorf("ordem = %s, %s; queria o mais recente primeiro", found[0].Path, found[1].Path)
	}
}
