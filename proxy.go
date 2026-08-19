package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Endpoint struct {
	Scheme string // "socks5", "http" ou "https"
	Host   string
	Port   int
}

func (e Endpoint) String() string {
	return fmt.Sprintf("%s://%s:%d", e.Scheme, e.Host, e.Port)
}

func (e Endpoint) address() string {
	return net.JoinHostPort(e.Host, strconv.Itoa(e.Port))
}

var proxyRulesRe = regexp.MustCompile(`^(socks5|https?)://([a-z0-9.-]{1,253}):(\d{1,5})$`)

func ParseProxy(raw string) (Endpoint, bool) {
	match := proxyRulesRe.FindStringSubmatch(strings.TrimSpace(strings.ToLower(raw)))
	if match == nil {
		return Endpoint{}, false
	}

	port, err := strconv.Atoi(match[3])
	if err != nil || port < 1 || port > 65535 {
		return Endpoint{}, false
	}

	return Endpoint{Scheme: match[1], Host: match[2], Port: port}, true
}

func OpenTunnel(e Endpoint, host string, port int, timeout time.Duration) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", e.address(), timeout)
	if err != nil {
		return nil, err
	}

	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		conn.Close()
		return nil, err
	}

	if e.Scheme == "socks5" {
		err = negotiateSOCKS5(conn, host, port)
	} else {
		err = negotiateConnect(conn, host, port)
	}
	if err != nil {
		conn.Close()
		return nil, err
	}

	if err := conn.SetDeadline(time.Time{}); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func negotiateSOCKS5(conn net.Conn, host string, port int) error {
	if len(host) > 255 {
		return errors.New("host longo demais para SOCKS5")
	}

	if _, err := conn.Write([]byte{5, 1, 0}); err != nil {
		return err
	}

	greeting := make([]byte, 2)
	if _, err := io.ReadFull(conn, greeting); err != nil {
		return err
	}
	if greeting[0] != 5 || greeting[1] != 0 {
		return fmt.Errorf("proxy exigiu autenticacao (metodo 0x%02x)", greeting[1])
	}

	request := []byte{5, 1, 0, 3, byte(len(host))}
	request = append(request, host...)
	request = append(request, byte(port>>8), byte(port&0xff))
	if _, err := conn.Write(request); err != nil {
		return err
	}

	head := make([]byte, 4)
	if _, err := io.ReadFull(conn, head); err != nil {
		return err
	}
	if head[1] != 0 {
		return fmt.Errorf("proxy recusou o destino (codigo 0x%02x)", head[1])
	}

	// O endereço de ligação não interessa, mas precisa sair do buffer: o que sobrar aqui
	// vira lixo no começo do handshake TLS.
	var rest int
	switch head[3] {
	case 1:
		rest = 4 + 2
	case 4:
		rest = 16 + 2
	case 3:
		length := make([]byte, 1)
		if _, err := io.ReadFull(conn, length); err != nil {
			return err
		}
		rest = int(length[0]) + 2
	default:
		return fmt.Errorf("tipo de endereco desconhecido na resposta: 0x%02x", head[3])
	}

	if _, err := io.ReadFull(conn, make([]byte, rest)); err != nil {
		return err
	}
	return nil
}

func negotiateConnect(conn net.Conn, host string, port int) error {
	target := net.JoinHostPort(host, strconv.Itoa(port))
	request := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	if _, err := conn.Write([]byte(request)); err != nil {
		return err
	}

	// Byte a byte mesmo: ler em bloco engoliria o começo do TLS que o destino já pode ter
	// mandado junto.
	var head []byte
	buf := make([]byte, 1)
	for !strings.HasSuffix(string(head), "\r\n\r\n") {
		if _, err := io.ReadFull(conn, buf); err != nil {
			return err
		}
		head = append(head, buf[0])
		if len(head) > 8192 {
			return errors.New("cabecalho de resposta longo demais")
		}
	}

	status := string(head)
	if !strings.HasPrefix(status, "HTTP/1.1 200") && !strings.HasPrefix(status, "HTTP/1.0 200") {
		line, _, _ := strings.Cut(status, "\r\n")
		return fmt.Errorf("CONNECT recusado: %s", line)
	}
	return nil
}

// net/http não serve aqui: o socket já vem negociado pelo proxy e precisamos dele cru.
func readOverTLS(conn net.Conn, host, path string, timeout time.Duration) (string, error) {
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return "", err
	}

	client := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	if err := client.Handshake(); err != nil {
		return "", err
	}

	request := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: Mozilla/5.0\r\nAccept: */*\r\nConnection: close\r\n\r\n", path, host)
	if _, err := client.Write([]byte(request)); err != nil {
		return "", err
	}

	body, err := io.ReadAll(io.LimitReader(client, maxResponseBytes))
	if len(body) == 0 && err != nil {
		return "", err
	}
	return string(body), nil
}

const maxResponseBytes = 64 * 1024

// O teste vai até a resposta 200 porque proxy que só aceita a conexão TCP passa em
// qualquer verificação mais simples e derruba o Discord depois.
func Probe(e Endpoint, timeout time.Duration) (time.Duration, error) {
	started := time.Now()

	conn, err := OpenTunnel(e, discordHost, 443, timeout)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	response, err := readOverTLS(conn, discordHost, "/api/v9/gateway", timeout)
	if err != nil {
		return 0, err
	}
	if !strings.HasPrefix(response, "HTTP/1.1 200") {
		line, _, _ := strings.Cut(response, "\r\n")
		return 0, fmt.Errorf("resposta inesperada do Discord: %s", line)
	}

	return time.Since(started), nil
}

var countryRe = regexp.MustCompile(`"country_iso"\s*:\s*"([A-Za-z]{2})"|"country(?:Code)?"\s*:\s*"([A-Za-z]{2})"`)

func ExitCountry(e Endpoint, timeout time.Duration) (string, error) {
	conn, err := OpenTunnel(e, geoHost, 443, timeout)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	response, err := readOverTLS(conn, geoHost, "/json", timeout)
	if err != nil {
		return "", err
	}

	match := countryRe.FindStringSubmatch(response)
	if match == nil {
		return "", errors.New("nao achei o pais na resposta do servico de geolocalizacao")
	}
	if match[1] != "" {
		return strings.ToUpper(match[1]), nil
	}
	return strings.ToUpper(match[2]), nil
}

func Accepts(e Endpoint, excluded map[string]bool, timeout time.Duration) (string, error) {
	country, err := ExitCountry(e, timeout)
	if err != nil {
		return "", err
	}
	if excluded[country] {
		return country, fmt.Errorf("saida em %s, que esta na lista de recusados", country)
	}
	return country, nil
}
