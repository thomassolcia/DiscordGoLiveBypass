//go:build windows

package main

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strings"

	"golang.zx2c4.com/wireguard/windows/conf"
)

const (
	defaultMTU = 1420
	// Sem keepalive, o mapeamento do roteador de casa expira num silêncio mais longo e o
	// servidor perde o caminho de volta. Arquivo que já traz um valor fica com o dele.
	defaultKeepalive = 25
)

// O que dá para mostrar de um arquivo de VPN sem expor a chave dele.
type confSummary struct {
	Server   string `json:"server"`
	Endpoint string `json:"endpoint"`
	Address  string `json:"address"`
}

// parseConf lê o texto de um arquivo WireGuard e recusa o que não serviria para o túnel. O
// nome só aparece nas mensagens.
func parseConf(text, name string) (*conf.Config, error) {
	if len(text) > maxConfBytes {
		return nil, fmt.Errorf("o arquivo %s é grande demais para ser uma configuração WireGuard", name)
	}

	cfg, err := conf.FromWgQuickWithUnknownEncoding(text, adapterName)
	if err != nil {
		return nil, fmt.Errorf("o arquivo %s não é uma configuração WireGuard válida (%s). Gere outro no site da sua VPN", name, redactKeys(err.Error(), text))
	}
	if len(cfg.Peers) == 0 {
		return nil, fmt.Errorf("o arquivo %s não tem nenhum servidor, falta a seção [Peer]", name)
	}
	for i := range cfg.Peers {
		if cfg.Peers[i].Endpoint.IsEmpty() {
			return nil, fmt.Errorf("o arquivo %s não diz o endereço do servidor, falta o Endpoint no [Peer]", name)
		}
	}

	// O Discord conversa por IPv4. Um túnel só com IPv6 subiria sem erro e não levaria nada.
	hasIPv4 := false
	for _, address := range cfg.Interface.Addresses {
		if address.Addr().Is4() {
			hasIPv4 = true
		}
	}
	if !hasIPv4 {
		return nil, fmt.Errorf("o arquivo %s não tem um endereço IPv4 no Address do [Interface]", name)
	}

	if err := checkEndpoints(cfg, discordPrefixes); err != nil {
		return nil, err
	}
	return cfg, nil
}

func readConfFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("não consegui abrir o arquivo %s", path)
	}
	if info.Size() > maxConfBytes {
		return "", errors.New("o arquivo é grande demais para ser uma configuração WireGuard")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("não consegui ler o arquivo %s: %v", path, err)
	}
	return string(raw), nil
}

// O parser oficial repete na mensagem o trecho que recusou, e quando o problema é a chave
// esse trecho é a própria chave. Ela sai da mensagem antes de ir para a tela ou para o
// log. Valor curto demais não é chave de verdade e trocá-lo picotaria o resto do texto.
func redactKeys(message, raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		name, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "privatekey", "presharedkey":
		default:
			continue
		}
		if comment := strings.Index(value, "#"); comment >= 0 {
			value = value[:comment]
		}
		if value = strings.TrimSpace(value); len(value) >= 8 {
			message = strings.ReplaceAll(message, value, "<chave omitida>")
		}
	}
	return message
}

func summarize(cfg *conf.Config, text string) confSummary {
	summary := confSummary{Endpoint: cfg.Peers[0].Endpoint.String(), Server: serverLabel(text)}
	if summary.Server == "" {
		summary.Server = cfg.Peers[0].Endpoint.Host
	}
	for _, address := range cfg.Interface.Addresses {
		if address.Addr().Is4() {
			summary.Address = address.String()
			break
		}
	}
	return summary
}

// Se o servidor da VPN ficasse dentro das faixas do Discord, o próprio túnel tentaria
// passar por dentro dele mesmo e nada sairia.
func checkEndpoints(cfg *conf.Config, routes []netip.Prefix) error {
	for i := range cfg.Peers {
		addr, err := netip.ParseAddr(cfg.Peers[i].Endpoint.Host)
		if err == nil && covered(routes, addr) {
			return fmt.Errorf("o servidor da VPN (%s) fica dentro da rede do Discord, e o túnel não consegue passar por dentro dele mesmo. Use um arquivo de outro servidor", cfg.Peers[i].Endpoint.Host)
		}
	}
	return nil
}

// ValidateConf confere um arquivo antes de ele ir para o serviço.
func ValidateConf(path string) (confSummary, error) {
	text, err := readConfFile(path)
	if err != nil {
		return confSummary{}, err
	}
	cfg, err := parseConf(text, fileBase(path))
	if err != nil {
		return confSummary{}, err
	}
	return summarize(cfg, text), nil
}

func fileBase(path string) string {
	if i := strings.LastIndexAny(path, `\/`); i >= 0 {
		return path[i+1:]
	}
	return path
}
