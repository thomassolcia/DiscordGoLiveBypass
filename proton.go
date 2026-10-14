package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/ProtonMail/go-srp"
)

// O Proton não tem API pública para terceiros. Este cliente fala com a mesma API do app
// oficial para Linux e se apresenta como ele, do mesmo jeito que o protonvpn-wg-confgen
// (GPL-3.0), de onde vêm os caminhos, os códigos de erro e o formato do certificado. Se o
// Proton aposentar esta versão, o login para com o código 5003 e a saída é atualizar o
// programa; o caminho do arquivo .conf continua funcionando sem login nenhum.
const (
	protonAPI        = "https://vpn-api.proton.me"
	protonAppVersion = "linux-vpn@4.17.2"
	protonUserAgent  = "ProtonVPN/4.17.2 (Linux; Ubuntu)"

	protonCodeOK           = 1000
	protonCodeMulti        = 1001
	protonCodeWrongPass    = 8002
	protonCodeCaptcha      = 9001
	protonCodeTwoPassword  = 10013
	protonCodeOldVersion   = 5003
	protonCodeOldVersion2  = 5005
	protonCodeAccountGone  = 10002
	protonCodeAccountBlock = 10003

	protonTierFree      = 0
	protonFeatureSecure = 1
	protonFeatureTor    = 2

	// Um ano, como os arquivos que o painel do Proton gera. Se a conta não aceitar, o
	// pedido cai para um certificado de sessão, de no máximo sete dias.
	protonCertMinutes        = 525600
	protonSessionCertMinutes = 10080
)

var (
	errProtonTwoFactor = errors.New("a conta tem verificação em duas etapas: digite o código do aplicativo autenticador")
	errProtonNoServer  = errors.New("o Proton não devolveu nenhum servidor gratuito fora do Brasil disponível agora")
)

// ProtonCaptcha é o pedido de verificação humana do Proton, com o endereço da página que
// a resolve e o desafio que a resposta precisa trazer no começo.
type ProtonCaptcha struct {
	URL       string
	Challenge string
}

func (c *ProtonCaptcha) Error() string {
	return "o Proton pediu uma verificação de segurança"
}

type protonClient struct {
	http   *http.Client
	base   string
	uid    string
	access string
}

func newProtonClient() *protonClient {
	return &protonClient{http: &http.Client{Timeout: 20 * time.Second}, base: protonAPI}
}

type protonResponse struct {
	Code    int    `json:"Code"`
	Error   string `json:"Error"`
	Details struct {
		HumanVerificationToken string `json:"HumanVerificationToken"`
	} `json:"Details"`
}

func (c *protonClient) call(ctx context.Context, method, path string, body any, headers map[string]string, out any) error {
	var payload io.Reader = http.NoBody
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, payload)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-pm-appversion", protonAppVersion)
	req.Header.Set("User-Agent", protonUserAgent)
	if c.access != "" {
		req.Header.Set("Authorization", "Bearer "+c.access)
		req.Header.Set("x-pm-uid", c.uid)
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}

	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("não consegui falar com o Proton: %s", DescribeNetwork(err))
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 8*1024*1024))
	if err != nil {
		return err
	}

	// O Proton diz o que deu errado no corpo, com um código próprio. O status HTTP só
	// interessa quando nem JSON veio.
	var status protonResponse
	if err := json.Unmarshal(raw, &status); err != nil {
		return fmt.Errorf("o Proton respondeu algo inesperado (HTTP %d)", res.StatusCode)
	}
	if status.Code != protonCodeOK && status.Code != protonCodeMulti {
		return protonError(status, c.base)
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

func protonError(status protonResponse, base string) error {
	switch status.Code {
	case protonCodeCaptcha:
		query := url.Values{"Token": {status.Details.HumanVerificationToken}}
		return &ProtonCaptcha{
			URL:       base + "/core/v4/captcha?" + query.Encode(),
			Challenge: status.Details.HumanVerificationToken,
		}
	case protonCodeWrongPass:
		return errors.New("e-mail ou senha do Proton incorretos")
	case protonCodeTwoPassword:
		return errors.New("a conta usa o modo antigo de duas senhas, que este programa não aceita. Em account.proton.me, vá em Configurações, Conta e senha, e mude para o modo de uma senha")
	case protonCodeOldVersion, protonCodeOldVersion2:
		return errors.New("o Proton não aceita mais a versão de login que este programa usa. Atualize o DiscordGoLiveBypass ou use um arquivo .conf baixado no site")
	case protonCodeAccountGone, protonCodeAccountBlock:
		return errors.New("o Proton diz que esta conta está desativada")
	}
	if status.Error != "" {
		return fmt.Errorf("o Proton recusou o pedido: %s (código %d)", status.Error, status.Code)
	}
	return fmt.Errorf("o Proton recusou o pedido (código %d)", status.Code)
}

func normalizeTwoFactor(code string) (string, error) {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != 6 {
		return "", errors.New("o código de verificação tem 6 números")
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return "", errors.New("o código de verificação tem só números")
		}
	}
	return code, nil
}

// login faz o SRP do Proton: a senha nunca sai do computador, só a prova de que ela é
// conhecida. twoFactor pode vir vazio na primeira tentativa; se a conta exigir, a resposta
// é errProtonTwoFactor e a janela pede o código. hvToken é a resposta de um CAPTCHA.
func (c *protonClient) login(ctx context.Context, username, password, twoFactor, hvToken string) error {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return errors.New("preencha o e-mail e a senha da conta do Proton")
	}

	var info struct {
		Version         int    `json:"Version"`
		Modulus         string `json:"Modulus"`
		ServerEphemeral string `json:"ServerEphemeral"`
		Salt            string `json:"Salt"`
		SRPSession      string `json:"SRPSession"`
		TwoFA           struct {
			Enabled int `json:"Enabled"`
			TOTP    int `json:"TOTP"`
		} `json:"2FA"`
	}
	if err := c.call(ctx, http.MethodPost, "/auth/info", map[string]any{"Username": username}, nil, &info); err != nil {
		return err
	}

	auth, err := srp.NewAuth(info.Version, username, []byte(password), info.Salt, info.Modulus, info.ServerEphemeral)
	if err != nil {
		return fmt.Errorf("não consegui preparar o login seguro: %v", err)
	}
	proofs, err := auth.GenerateProofs(2048)
	if err != nil {
		return fmt.Errorf("não consegui preparar o login seguro: %v", err)
	}

	request := map[string]any{
		"Username":        username,
		"ClientEphemeral": base64.StdEncoding.EncodeToString(proofs.ClientEphemeral),
		"ClientProof":     base64.StdEncoding.EncodeToString(proofs.ClientProof),
		"SRPSession":      info.SRPSession,
	}
	if info.TwoFA.Enabled == 1 && info.TwoFA.TOTP == 1 {
		if twoFactor == "" {
			return errProtonTwoFactor
		}
		code, err := normalizeTwoFactor(twoFactor)
		if err != nil {
			return err
		}
		request["TwoFactorCode"] = code
	}

	headers := map[string]string{}
	if hvToken != "" {
		headers["x-pm-human-verification-token"] = hvToken
		headers["x-pm-human-verification-token-type"] = "captcha"
	}

	var session struct {
		UID         string   `json:"UID"`
		AccessToken string   `json:"AccessToken"`
		ServerProof string   `json:"ServerProof"`
		Scopes      []string `json:"Scopes"`
	}
	if err := c.call(ctx, http.MethodPost, "/auth", request, headers, &session); err != nil {
		return err
	}
	// Se a prova do servidor não bater, quem respondeu não é o Proton.
	if session.ServerProof != base64.StdEncoding.EncodeToString(proofs.ExpectedServerProof) {
		return errors.New("a resposta do login não veio do Proton, por segurança nada foi gerado")
	}
	c.uid, c.access = session.UID, session.AccessToken

	// Algumas contas só ganham acesso à VPN depois do código de duas etapas, mesmo quando
	// o /auth/info não avisou. O mesmo código serve aqui.
	if !hasScope(session.Scopes, "vpn") && hasScope(session.Scopes, "twofactor") {
		if twoFactor == "" {
			c.logout(ctx)
			return errProtonTwoFactor
		}
		code, err := normalizeTwoFactor(twoFactor)
		if err != nil {
			return err
		}
		var upgraded struct {
			Scopes []string `json:"Scopes"`
		}
		if err := c.call(ctx, http.MethodPost, "/auth/2fa", map[string]any{"TwoFactorCode": code}, nil, &upgraded); err != nil {
			return err
		}
		if !hasScope(upgraded.Scopes, "vpn") {
			return errors.New("o Proton não liberou o acesso à VPN para esta sessão")
		}
	}
	return nil
}

func hasScope(scopes []string, want string) bool {
	for _, scope := range scopes {
		if scope == want {
			return true
		}
	}
	return false
}

// A sessão só existe para gerar o arquivo. Encerrar logo depois não deixa nada válido
// para trás, nem no computador nem na conta.
func (c *protonClient) logout(ctx context.Context) {
	if c.access == "" {
		return
	}
	c.call(ctx, http.MethodDelete, "/auth", nil, nil, nil)
	c.uid, c.access = "", ""
}

type protonServer struct {
	Name      string
	Country   string
	City      string
	Score     float64
	Load      int
	EntryIP   string
	PublicKey string
}

type protonLogical struct {
	Name        string  `json:"Name"`
	ExitCountry string  `json:"ExitCountry"`
	City        string  `json:"City"`
	Tier        int     `json:"Tier"`
	Features    int     `json:"Features"`
	Score       float64 `json:"Score"`
	Load        int     `json:"Load"`
	Status      int     `json:"Status"`
	Servers     []struct {
		EntryIP         string `json:"EntryIP"`
		Status          int    `json:"Status"`
		X25519PublicKey string `json:"X25519PublicKey"`
	} `json:"Servers"`
}

// freeServers fica só com o que serve aqui: plano gratuito, ligado, fora do Brasil e sem
// rotas especiais. O Proton calcula o Score a partir de quem pergunta, então o menor é o
// melhor para esta conexão, como no "conexão rápida" do app dele.
func freeServers(logicals []protonLogical) []protonServer {
	var list []protonServer
	for _, logical := range logicals {
		if logical.Tier != protonTierFree || logical.Status != 1 || strings.EqualFold(logical.ExitCountry, "BR") ||
			logical.Features&(protonFeatureSecure|protonFeatureTor) != 0 {
			continue
		}
		for _, physical := range logical.Servers {
			if physical.Status != 1 || physical.EntryIP == "" || physical.X25519PublicKey == "" {
				continue
			}
			list = append(list, protonServer{
				Name: logical.Name, Country: strings.ToUpper(logical.ExitCountry), City: logical.City,
				Score: logical.Score, Load: logical.Load, EntryIP: physical.EntryIP, PublicKey: physical.X25519PublicKey,
			})
			break
		}
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].Score < list[j].Score })
	return list
}

func (c *protonClient) servers(ctx context.Context) ([]protonServer, error) {
	var response struct {
		LogicalServers []protonLogical `json:"LogicalServers"`
	}
	if err := c.call(ctx, http.MethodGet, "/vpn/v1/logicals", nil, nil, &response); err != nil {
		return nil, err
	}
	list := freeServers(response.LogicalServers)
	if len(list) == 0 {
		return nil, errProtonNoServer
	}
	return list, nil
}

// pickServer escolhe o melhor do país pedido, ou o melhor de todos sem país.
func pickServer(list []protonServer, country string) (protonServer, bool) {
	for _, server := range list {
		if country == "" || strings.EqualFold(server.Country, country) {
			return server, true
		}
	}
	return protonServer{}, false
}

// O Proton certifica uma chave Ed25519, e o WireGuard usa a X25519 que sai dela: o
// escalar é o começo do SHA-512 da semente, com os bits ajustados como manda a curva.
func x25519FromEd25519(private ed25519.PrivateKey) []byte {
	digest := sha512.Sum512(private.Seed())
	scalar := digest[:32]
	scalar[0] &= 248
	scalar[31] &= 127
	scalar[31] |= 64
	return scalar
}

func ed25519PublicPEM(public ed25519.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(public)
	if err != nil {
		return "", err
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})), nil
}

func (c *protonClient) certificate(ctx context.Context, public ed25519.PublicKey, device string) (time.Time, error) {
	key, err := ed25519PublicPEM(public)
	if err != nil {
		return time.Time{}, err
	}
	features := map[string]any{"NetShieldLevel": 0, "RandomNAT": true, "PortForwarding": false, "SplitTCP": true}

	var cert struct {
		ExpirationTime int64 `json:"ExpirationTime"`
	}
	err = c.call(ctx, http.MethodPost, "/vpn/v1/certificate", map[string]any{
		"ClientPublicKey": key, "ClientPublicKeyMode": "EC", "Mode": "persistent", "DeviceName": device,
		"Duration": fmt.Sprintf("%d min", protonCertMinutes), "Features": features,
	}, nil, &cert)
	if err != nil {
		var captcha *ProtonCaptcha
		if errors.As(err, &captcha) {
			return time.Time{}, err
		}
		err = c.call(ctx, http.MethodPost, "/vpn/v1/certificate", map[string]any{
			"ClientPublicKey": key, "ClientPublicKeyMode": "EC",
			"Duration": fmt.Sprintf("%d min", protonSessionCertMinutes), "Features": features,
		}, nil, &cert)
		if err != nil {
			return time.Time{}, err
		}
	}
	return time.Unix(cert.ExpirationTime, 0), nil
}

// buildProtonConf monta o mesmo arquivo que o painel do Proton entrega. O nome do servidor
// vai num comentário logo depois do [Peer], onde o resto do programa já procura.
func buildProtonConf(server protonServer, privateKey []byte, expires time.Time) string {
	var b strings.Builder
	b.WriteString("# Gerado pelo DiscordGoLiveBypass com a sua conta do Proton VPN\n")
	if !expires.IsZero() {
		fmt.Fprintf(&b, "# Valido ate %s\n", expires.Format("02/01/2006"))
	}
	b.WriteString("[Interface]\n")
	fmt.Fprintf(&b, "PrivateKey = %s\n", base64.StdEncoding.EncodeToString(privateKey))
	b.WriteString("Address = 10.2.0.2/32\nDNS = 10.2.0.1\n\n[Peer]\n")
	fmt.Fprintf(&b, "# %s\n", server.Name)
	fmt.Fprintf(&b, "PublicKey = %s\n", server.PublicKey)
	b.WriteString("AllowedIPs = 0.0.0.0/0, ::/0\n")
	fmt.Fprintf(&b, "Endpoint = %s:51820\n", server.EntryIP)
	b.WriteString("PersistentKeepalive = 25\n")
	return b.String()
}

// generateConf usa a sessão já aberta para gerar um par de chaves, certificá-lo no Proton
// e montar o arquivo, e encerra a sessão no fim, dê certo ou não.
func (c *protonClient) generateConf(ctx context.Context, server protonServer) (string, time.Time, error) {
	defer c.logout(ctx)

	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", time.Time{}, err
	}
	device := "DiscordGoLiveBypass " + time.Now().Format("02-01-2006 15h04")
	expires, err := c.certificate(ctx, public, device)
	if err != nil {
		return "", time.Time{}, err
	}
	return buildProtonConf(server, x25519FromEd25519(private), expires), expires, nil
}
