package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
)

// Uma linha de JSON por pedido e uma por resposta, uma conexão por pedido. Simples de ler
// num log e sem estado para dar errado entre uma chamada e outra.
const (
	pipePath      = `\\.\pipe\DiscordGoLiveBypass`
	maxIPCMessage = 128 * 1024

	cmdStatus  = "status"
	cmdSetConf = "set_conf"
	cmdEnable  = "enable"
	cmdDisable = "disable"
	cmdSetMode = "set_mode"

	cmdSetAutostart = "set_autostart"
)

type ipcRequest struct {
	Cmd       string `json:"cmd"`
	Conf      string `json:"conf,omitempty"`
	Mode      string `json:"mode,omitempty"`
	Autostart bool   `json:"autostart,omitempty"`
}

// ServiceStatus é tudo o que a janela sabe do túnel. Nunca carrega a chave da VPN.
type ServiceStatus struct {
	Version   string `json:"version"`
	Enabled   bool   `json:"enabled"`
	HasConf   bool   `json:"has_conf"`
	State     string `json:"state"`
	Error     string `json:"error,omitempty"`
	Server    string `json:"server,omitempty"`
	Endpoint  string `json:"endpoint,omitempty"`
	Mode      string `json:"mode,omitempty"`
	Routes    int    `json:"routes"`
	Streams   int    `json:"streams"`
	Handshake int64  `json:"handshake_s"`
	Rx        uint64 `json:"rx"`
	Tx        uint64 `json:"tx"`
	// Ausente num serviço de antes da 2.3, que não sabe deixar de iniciar com o Windows.
	Autostart *bool `json:"autostart,omitempty"`
}

type ipcResponse struct {
	OK     bool          `json:"ok"`
	Error  string        `json:"error,omitempty"`
	Status ServiceStatus `json:"status"`
}

var errMessageTooLarge = errors.New("mensagem grande demais no canal do serviço")

func writeMessage(w io.Writer, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(raw) > maxIPCMessage {
		return errMessageTooLarge
	}
	_, err = w.Write(append(raw, '\n'))
	return err
}

// Lê até o fim da linha sem deixar um cliente mandar um pedido sem fim.
func readMessage(r io.Reader, v any) error {
	reader := bufio.NewReaderSize(io.LimitReader(r, maxIPCMessage+1), 4096)
	line, err := reader.ReadBytes('\n')
	if len(line) > maxIPCMessage {
		return errMessageTooLarge
	}
	if err != nil && (err != io.EOF || len(line) == 0) {
		return err
	}
	return json.Unmarshal(line, v)
}
