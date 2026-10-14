//go:build windows

package main

import (
	"errors"
	"log"
	"net"
	"time"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/ipc/namedpipe"
)

var (
	errServiceAbsent = errors.New("o serviço do túnel não está rodando")
	errServiceDenied = errors.New("este usuário não tem permissão para controlar o túnel. Ele foi instalado por outra conta do Windows")
)

// Dono explícito, o sistema: é isso que a janela confere antes de mandar a chave da VPN.
// Além do sistema e dos administradores, só o usuário que instalou conversa com o serviço.
func listenIPC(ownerSID string) (net.Listener, error) {
	sddl := "O:SYD:P(A;;GA;;;SY)(A;;GA;;;BA)"
	if sid, err := windows.StringToSid(ownerSID); err == nil && sid.IsValid() {
		sddl += "(A;;GRGW;;;" + sid.String() + ")"
	}
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return nil, err
	}
	return (&namedpipe.ListenConfig{SecurityDescriptor: sd}).Listen(pipePath)
}

func serveIPC(listener net.Listener, handle func(ipcRequest) ipcResponse) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go func() {
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(30 * time.Second))

			var req ipcRequest
			if err := readMessage(conn, &req); err != nil {
				log.Printf("pedido ilegível no canal: %v", err)
				return
			}
			writeMessage(conn, handle(req))
		}()
	}
}

// Sem conferir o dono, qualquer programa do usuário poderia criar um canal com esse nome
// antes do serviço e receber a chave da VPN quando a janela a enviasse.
func callService(req ipcRequest, timeout time.Duration) (ipcResponse, error) {
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return ipcResponse{}, err
	}

	conn, err := (&namedpipe.DialConfig{ExpectedOwner: system}).DialTimeout(pipePath, 2*time.Second)
	switch {
	case errors.Is(err, windows.ERROR_FILE_NOT_FOUND):
		return ipcResponse{}, errServiceAbsent
	case errors.Is(err, windows.ERROR_ACCESS_DENIED):
		return ipcResponse{}, errServiceDenied
	case err != nil:
		return ipcResponse{}, err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))

	if err := writeMessage(conn, req); err != nil {
		return ipcResponse{}, err
	}
	var resp ipcResponse
	if err := readMessage(conn, &resp); err != nil {
		return ipcResponse{}, err
	}
	if !resp.OK {
		return resp, errors.New(resp.Error)
	}
	return resp, nil
}
