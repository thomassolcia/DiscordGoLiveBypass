package main

import (
	"net/netip"
	"strings"
	"testing"
	"time"
)

type fonteFalsa struct {
	linhas  []string
	fechada bool
}

func (f *fonteFalsa) poll(now time.Time, line func(string)) {
	for _, l := range f.linhas {
		line(l)
	}
	f.linhas = nil
}

func (f *fonteFalsa) close() { f.fechada = true }

func avisou(w *mediaWatch) bool {
	select {
	case <-w.changed:
		return true
	default:
		return false
	}
}

func TestLineBufferJuntaPedacos(t *testing.T) {
	var b lineBuffer
	var linhas []string
	junta := func(l string) { linhas = append(linhas, l) }
	b.feed([]byte("um\r\ndo"), junta)
	b.feed([]byte("is\ntr"), junta)
	b.feed([]byte("es\n"), junta)
	if strings.Join(linhas, "|") != "um|dois|tres" {
		t.Errorf("linhas = %q", linhas)
	}

	b.feed([]byte(strings.Repeat("x", maxLogLine+1)), junta)
	if len(b.partial) != 0 {
		t.Errorf("guardou %d bytes de uma linha sem fim", len(b.partial))
	}
}

// Do log até as rotas, passando pelo aviso que acorda o motor.
func TestMediaWatchDoLogAteAsRotas(t *testing.T) {
	fonte := &fonteFalsa{linhas: []string{linhaVozConectando, linhaVozCriando, linhaVozConectado}}
	w := newMediaWatch([]lineSource{fonte})
	agora := time.Now()

	w.step(agora)
	if avisou(w) {
		t.Error("so a voz conectando avisou mudanca de rota")
	}

	fonte.linhas = []string{linhaInicio, linhaConectando}
	w.step(agora)
	if !avisou(w) {
		t.Fatal("o anuncio da transmissao nao avisou o motor")
	}
	rotas, abertas := w.state()
	if !covered(rotas, servidorStream) || covered(rotas, servidorVoz) || abertas != 0 {
		t.Errorf("esperando o socket: rotas = %v, abertas = %d", rotas, abertas)
	}

	fonte.linhas = []string{linhaCriando, linhaConectado}
	w.step(agora)
	rotas, abertas = w.state()
	if !avisou(w) || len(rotas) != 1 || rotas[0] != netip.MustParsePrefix("104.29.152.78/32") || abertas != 1 {
		t.Errorf("transmissao aberta: rotas = %v, abertas = %d", rotas, abertas)
	}

	fonte.linhas = []string{linhaFim}
	w.step(agora)
	if rotas, abertas = w.state(); !avisou(w) || len(rotas) != 0 || abertas != 0 {
		t.Errorf("depois do fim: rotas = %v, abertas = %d", rotas, abertas)
	}
}

func TestMediaWatchDesisteSozinho(t *testing.T) {
	fonte := &fonteFalsa{linhas: []string{linhaInicio}}
	w := newMediaWatch([]lineSource{fonte})
	agora := time.Now()
	w.step(agora)
	avisou(w)

	w.step(agora.Add(streamPendingTimeout + time.Second))
	if !avisou(w) {
		t.Fatal("o fim da espera nao avisou o motor")
	}
	if rotas, _ := w.state(); len(rotas) != 0 {
		t.Errorf("rotas = %v depois do prazo", rotas)
	}
}

func TestMediaWatchSemLogNaoSabeSeparar(t *testing.T) {
	if newMediaWatch(nil).available() {
		t.Error("sem log nenhum, available devia ser falso")
	}
}
