package main

import (
	"errors"
	"net"
	"net/url"
	"os"
	"strings"
	"syscall"
	"testing"
)

type fakeTimeout struct{}

func (fakeTimeout) Error() string   { return "i/o timeout" }
func (fakeTimeout) Timeout() bool   { return true }
func (fakeTimeout) Temporary() bool { return true }

// O erro chega como o net/http monta: url.Error sobre OpSyscall sobre o codigo do sistema.
func dialFailure(cause error) error {
	return &url.Error{
		Op:  "Get",
		URL: traceURL,
		Err: &net.OpError{
			Op:   "dial",
			Net:  "tcp",
			Addr: &net.TCPAddr{IP: net.IPv4(162, 159, 128, 233), Port: 443},
			Err:  os.NewSyscallError("connectex", cause),
		},
	}
}

func TestDescribeNetworkBlockedByMachine(t *testing.T) {
	for name, cause := range map[string]error{
		"windows": winSocketForbidden,
		"outros":  syscall.EACCES,
	} {
		got := DescribeNetwork(dialFailure(cause))
		if !strings.Contains(got, "antivirus ou firewall") {
			t.Errorf("%s: DescribeNetwork = %q, queria a dica de antivirus", name, got)
		}
		if strings.Contains(got, "cdn-cgi") {
			t.Errorf("%s: DescribeNetwork = %q, ainda carrega a URL crua", name, got)
		}
	}
}

func TestDescribeNetworkOutrosCasos(t *testing.T) {
	dns := &url.Error{Op: "Get", URL: traceURL, Err: &net.DNSError{Err: "no such host", Name: "discord.com"}}
	if got := DescribeNetwork(dns); !strings.Contains(got, "discord.com") || !strings.Contains(got, "DNS") {
		t.Errorf("DescribeNetwork(dns) = %q, queria falar de DNS e do endereco", got)
	}

	if got := DescribeNetwork(dialFailure(fakeTimeout{})); !strings.Contains(got, "demorou demais") {
		t.Errorf("DescribeNetwork(timeout) = %q, queria falar de demora", got)
	}

	if got := DescribeNetwork(dialFailure(syscall.ECONNREFUSED)); !strings.Contains(got, "recusada do outro lado") {
		t.Errorf("DescribeNetwork(recusa) = %q, queria falar de recusa remota", got)
	}

	// Sem causa conhecida sobra a mensagem de dentro, mas ja sem a URL.
	plain := &url.Error{Op: "Get", URL: traceURL, Err: errors.New("o Discord respondeu HTTP 503")}
	if got := DescribeNetwork(plain); got != "o Discord respondeu HTTP 503" {
		t.Errorf("DescribeNetwork(generico) = %q", got)
	}

	if got := DescribeNetwork(nil); got != "" {
		t.Errorf("DescribeNetwork(nil) = %q, queria vazio", got)
	}
}
