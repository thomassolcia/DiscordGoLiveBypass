package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestMessageRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	sent := ipcRequest{Cmd: cmdSetConf, Conf: sampleConf}
	if err := writeMessage(&buf, sent); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(buf.String(), "\n") || strings.Count(buf.String(), "\n") != 1 {
		t.Fatalf("a mensagem deveria ser uma linha so: %q", buf.String())
	}

	var got ipcRequest
	if err := readMessage(&buf, &got); err != nil {
		t.Fatal(err)
	}
	if got != sent {
		t.Errorf("readMessage = %+v, queria %+v", got, sent)
	}
}

func TestReadMessageRecusaPedidoSemFim(t *testing.T) {
	huge := strings.Repeat("a", maxIPCMessage+10)
	var got ipcRequest
	err := readMessage(strings.NewReader(huge), &got)
	if !errors.Is(err, errMessageTooLarge) {
		t.Errorf("readMessage(enorme) = %v, queria errMessageTooLarge", err)
	}
}
