package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ProtonMail/go-srp"
)

// Vetor calculado pela go-vpn-lib, a biblioteca do Proton, com a mesma semente. Conferido
// em 200 chaves aleatórias antes de fixar este.
func TestX25519FromEd25519(t *testing.T) {
	private := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, 32))
	got := base64.StdEncoding.EncodeToString(x25519FromEd25519(private))
	if want := "KK05/v1/o+IAqcYm7vWZ5hotBVxIqCiKTn5MS8o5KHg="; got != want {
		t.Errorf("x25519FromEd25519 = %s, queria %s", got, want)
	}
}

func logical(name, country string, tier, features, status int, score float64, ip string) protonLogical {
	l := protonLogical{Name: name, ExitCountry: country, Tier: tier, Features: features, Status: status, Score: score}
	l.Servers = append(l.Servers, struct {
		EntryIP         string `json:"EntryIP"`
		Status          int    `json:"Status"`
		X25519PublicKey string `json:"X25519PublicKey"`
	}{ip, 1, "CAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAg="})
	return l
}

func TestFreeServers(t *testing.T) {
	list := freeServers([]protonLogical{
		logical("US-FREE#2", "US", protonTierFree, 0, 1, 2.5, "203.0.113.2"),
		logical("BR-FREE#1", "BR", protonTierFree, 0, 1, 0.1, "203.0.113.9"),
		logical("US#10", "US", 2, 0, 1, 0.5, "203.0.113.10"),
		logical("NL-FREE#1", "NL", protonTierFree, 0, 0, 0.2, "203.0.113.11"),
		logical("CH-SC", "CH", protonTierFree, protonFeatureSecure, 1, 0.3, "203.0.113.12"),
		logical("JP-FREE#3", "jp", protonTierFree, 0, 1, 1.1, "203.0.113.3"),
	})

	if len(list) != 2 {
		t.Fatalf("freeServers ficou com %d, queria só US-FREE#2 e JP-FREE#3: %+v", len(list), list)
	}
	if list[0].Name != "JP-FREE#3" || list[0].Country != "JP" || list[1].Name != "US-FREE#2" {
		t.Errorf("ordem = %s, %s; queria o menor Score primeiro", list[0].Name, list[1].Name)
	}

	if best, ok := pickServer(list, ""); !ok || best.Name != "JP-FREE#3" {
		t.Errorf("pickServer automático = %+v", best)
	}
	if us, ok := pickServer(list, "us"); !ok || us.Name != "US-FREE#2" {
		t.Errorf("pickServer(us) = %+v", us)
	}
	if _, ok := pickServer(list, "DE"); ok {
		t.Error("pickServer(DE) deveria falhar sem servidor alemão")
	}
}

func TestBuildProtonConf(t *testing.T) {
	server := protonServer{Name: "US-FREE#57", Country: "US", EntryIP: "203.0.113.57", PublicKey: "CAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAg="}
	text := buildProtonConf(server, bytes.Repeat([]byte{7}, 32), time.Date(2027, 10, 14, 0, 0, 0, 0, time.UTC))

	for _, want := range []string{"Valido ate 14/10/2027", "Address = 10.2.0.2/32", "Endpoint = 203.0.113.57:51820", "PersistentKeepalive = 25"} {
		if !strings.Contains(text, want) {
			t.Errorf("o arquivo gerado não tem %q:\n%s", want, text)
		}
	}
	if !looksLikeWireGuard(text) {
		t.Error("o arquivo gerado não parece WireGuard para a busca de arquivos")
	}
	if got := serverLabel(text); got != "US-FREE#57" {
		t.Errorf("serverLabel = %q, a janela não vai mostrar o nome do servidor", got)
	}
}

func TestNormalizeTwoFactor(t *testing.T) {
	if code, err := normalizeTwoFactor(" 123 456 "); err != nil || code != "123456" {
		t.Errorf("normalizeTwoFactor = %q, %v", code, err)
	}
	for _, bad := range []string{"", "12345", "12345a", "1234567"} {
		if _, err := normalizeTwoFactor(bad); err == nil {
			t.Errorf("normalizeTwoFactor(%q) deveria recusar", bad)
		}
	}
}

func TestProtonErrors(t *testing.T) {
	var captcha *ProtonCaptcha
	err := protonError(protonResponse{Code: protonCodeCaptcha, Details: struct {
		HumanVerificationToken string `json:"HumanVerificationToken"`
	}{"desafio+/=1"}}, protonAPI)
	if !errors.As(err, &captcha) {
		t.Fatalf("9001 deveria virar ProtonCaptcha, veio %v", err)
	}
	if captcha.Challenge != "desafio+/=1" || captcha.URL != protonAPI+"/core/v4/captcha?Token=desafio%2B%2F%3D1" {
		t.Errorf("captcha = %+v", captcha)
	}

	for code, want := range map[int]string{
		protonCodeWrongPass:   "incorretos",
		protonCodeTwoPassword: "duas senhas",
		protonCodeOldVersion:  "Atualize",
		12345:                 "código 12345",
	} {
		if err := protonError(protonResponse{Code: code}, protonAPI); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("código %d virou %v, queria algo com %q", code, err, want)
		}
	}
}

// Um Proton de mentira: o certificado persistente é recusado e o de sessão aceito, para
// conferir que o pedido cai para o plano B e que a sessão é encerrada no fim.
func TestGenerateConfCaiParaCertificadoDeSessao(t *testing.T) {
	var pedidos []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pedidos = append(pedidos, r.Method+" "+r.URL.Path)
		if r.Header.Get("x-pm-appversion") != protonAppVersion || r.Header.Get("Authorization") != "Bearer token" {
			t.Errorf("cabeçalhos faltando em %s %s", r.Method, r.URL.Path)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/vpn/v1/certificate":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["Mode"] == "persistent" {
				json.NewEncoder(w).Encode(map[string]any{"Code": 2001, "Error": "não permitido"})
				return
			}
			if !strings.Contains(body["ClientPublicKey"].(string), "BEGIN PUBLIC KEY") {
				t.Error("a chave pública não foi em PEM")
			}
			json.NewEncoder(w).Encode(map[string]any{"Code": 1000, "ExpirationTime": time.Now().Add(7 * 24 * time.Hour).Unix()})
		case r.Method == http.MethodDelete && r.URL.Path == "/auth":
			json.NewEncoder(w).Encode(map[string]any{"Code": 1000})
		default:
			t.Errorf("pedido inesperado %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := &protonClient{http: server.Client(), base: server.URL, uid: "uid", access: "token"}
	text, expires, err := client.generateConf(context.Background(), protonServer{Name: "NL-FREE#1", EntryIP: "203.0.113.1", PublicKey: "CAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAg="})
	if err != nil {
		t.Fatalf("generateConf: %v", err)
	}
	if time.Until(expires) < 6*24*time.Hour || !strings.Contains(text, "# NL-FREE#1") {
		t.Errorf("arquivo ou validade errados: %v\n%s", expires, text)
	}
	if want := "POST /vpn/v1/certificate,POST /vpn/v1/certificate,DELETE /auth"; strings.Join(pedidos, ",") != want {
		t.Errorf("pedidos = %v, queria %s", pedidos, want)
	}
	if client.access != "" {
		t.Error("a sessão continuou guardada depois de gerar o arquivo")
	}
}

func TestCallRespostaQueNaoEJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("<html>gateway</html>"))
	}))
	defer server.Close()

	client := &protonClient{http: server.Client(), base: server.URL}
	err := client.call(context.Background(), http.MethodGet, "/vpn/v1/logicals", nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Errorf("call = %v, queria falar do HTTP 502", err)
	}
}

// Fala com o Proton de verdade, mas só pede os parâmetros de login de um usuário que não
// existe: nenhuma conta é usada. Pega cedo o dia em que o Proton recusar a versão de app
// (código 5003) ou trocar a assinatura do módulo do SRP. DGLB_REDE=1 go test -run AoVivo.
func TestProtonAuthInfoAoVivo(t *testing.T) {
	if os.Getenv("DGLB_REDE") == "" {
		t.Skip("defina DGLB_REDE=1 para consultar o Proton")
	}
	client := newProtonClient()
	var info struct {
		Version         int    `json:"Version"`
		Modulus         string `json:"Modulus"`
		ServerEphemeral string `json:"ServerEphemeral"`
		Salt            string `json:"Salt"`
	}
	if err := client.call(context.Background(), http.MethodPost, "/auth/info", map[string]any{"Username": "teste-dglb-inexistente"}, nil, &info); err != nil {
		t.Fatalf("auth/info: %v", err)
	}
	auth, err := srp.NewAuth(info.Version, "teste-dglb-inexistente", []byte("senha de teste"), info.Salt, info.Modulus, info.ServerEphemeral)
	if err != nil {
		t.Fatalf("o módulo do SRP não passou na verificação: %v", err)
	}
	if _, err := auth.GenerateProofs(2048); err != nil {
		t.Fatalf("GenerateProofs: %v", err)
	}
}
