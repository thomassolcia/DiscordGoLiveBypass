package main

import (
	"testing"
	"time"
)

func TestRouteDecision(t *testing.T) {
	cases := []struct {
		name   string
		routed bool
		age    time.Duration
		want   routeAction
	}{
		{"primeiro handshake chegou", false, 2 * time.Second, addRoutes},
		{"ainda sem handshake", false, -1, keepRoutes},
		{"rodando normal", true, 90 * time.Second, keepRoutes},
		{"rekey atrasado mas dentro do prazo", true, handshakeStale - time.Second, keepRoutes},
		{"servidor caiu", true, handshakeStale, dropRoutes},
		{"servidor voltou", false, 5 * time.Second, addRoutes},
		{"continua fora", false, 10 * time.Minute, keepRoutes},
	}
	for _, c := range cases {
		if got := routeDecision(c.routed, c.age); got != c.want {
			t.Errorf("%s: routeDecision(%v, %v) = %v, queria %v", c.name, c.routed, c.age, got, c.want)
		}
	}
}

func TestSessionState(t *testing.T) {
	cases := []struct {
		name       string
		routed     bool
		age, since time.Duration
		want       string
	}{
		{"recem ligado", false, -1, 5 * time.Second, stateConnecting},
		{"nenhum handshake depois do prazo", false, -1, firstHandshakeWait, stateNoServer},
		{"com rotas", true, 30 * time.Second, 10 * time.Minute, stateOn},
		{"rotas tiradas por servidor fora", false, 5 * time.Minute, 20 * time.Minute, stateNoServer},
	}
	for _, c := range cases {
		if got := sessionState(c.routed, c.age, c.since); got != c.want {
			t.Errorf("%s: sessionState = %q, queria %q", c.name, got, c.want)
		}
	}
}
