package main

import "time"

// Estados do túnel, os mesmos textos que a janela recebe do serviço.
const (
	stateOff        = "desligado"
	stateConnecting = "conectando"
	stateOn         = "ligado"
	stateNoServer   = "sem_resposta"
	stateFailed     = "erro"
)

const (
	// Com keepalive de 25 s, o WireGuard refaz o handshake a cada dois minutos enquanto o
	// servidor responde. Três minutos sem handshake novo é servidor fora do ar, não atraso.
	handshakeStale = 3 * time.Minute
	// Quanto o túnel espera o primeiro handshake antes de dizer que o servidor não respondeu.
	firstHandshakeWait = 30 * time.Second
)

type routeAction int

const (
	keepRoutes routeAction = iota
	addRoutes
	dropRoutes
)

// handshakeAge negativo quer dizer que nunca houve handshake.
func healthy(handshakeAge time.Duration) bool {
	return handshakeAge >= 0 && handshakeAge < handshakeStale
}

func routeDecision(routed bool, handshakeAge time.Duration) routeAction {
	switch ok := healthy(handshakeAge); {
	case ok && !routed:
		return addRoutes
	case !ok && routed:
		return dropRoutes
	}
	return keepRoutes
}

func sessionState(routed bool, handshakeAge, sinceStart time.Duration) string {
	switch {
	case routed:
		return stateOn
	case handshakeAge < 0 && sinceStart < firstHandshakeWait:
		return stateConnecting
	}
	return stateNoServer
}
