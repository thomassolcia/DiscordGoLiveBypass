package main

import (
	"bytes"
	"context"
	"log"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// O log chega ao disco 20 a 30 ms depois do evento, e o socket da transmissão abre quase
// meio segundo depois do anúncio. Ler a cada 50 ms deixa folga sem pesar.
const logPollInterval = 50 * time.Millisecond

// Linha maior que isso não é das que interessam, e guardar o pedaço dela sem fim só
// gastaria memória.
const maxLogLine = 64 * 1024

// lineSource entrega as linhas novas de um log a cada chamada de poll.
type lineSource interface {
	poll(now time.Time, line func(string))
	close()
}

// lineBuffer junta os pedaços lidos do arquivo em linhas inteiras.
type lineBuffer struct {
	partial []byte
}

func (b *lineBuffer) feed(chunk []byte, line func(string)) {
	data := append(b.partial, chunk...)
	for {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			break
		}
		line(strings.TrimRight(string(data[:i]), "\r"))
		data = data[i+1:]
	}
	if len(data) > maxLogLine {
		data = nil
	}
	b.partial = append([]byte(nil), data...)
}

func (b *lineBuffer) reset() { b.partial = nil }

// mediaWatch acompanha os logs do Discord e diz o que precisa estar no túnel. Vive enquanto
// o túnel estiver ligado, e não por sessão: o túnel reconectando não esquece a transmissão
// que já estava aberta.
type mediaWatch struct {
	mu      sync.Mutex
	split   *splitter
	sources []lineSource
	changed chan struct{}
}

func newMediaWatch(sources []lineSource) *mediaWatch {
	return &mediaWatch{split: newSplitter(), sources: sources, changed: make(chan struct{}, 1)}
}

// Sem log nenhum para ler, não há como saber quando uma transmissão abre.
func (w *mediaWatch) available() bool { return len(w.sources) > 0 }

func (w *mediaWatch) state() (routes []netip.Prefix, streams int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.split.routes(), len(w.split.streamAddrs())
}

func (w *mediaWatch) run(ctx context.Context) {
	defer func() {
		for _, source := range w.sources {
			source.close()
		}
	}()
	ticker := time.NewTicker(logPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			w.step(now)
		}
	}
}

func (w *mediaWatch) step(now time.Time) {
	changed := false
	for _, source := range w.sources {
		source.poll(now, func(line string) {
			if ev, ok := parseRTCLine(line); ok && w.handle(ev, now) {
				changed = true
			}
		})
	}
	w.mu.Lock()
	if w.split.expire(now) {
		changed = true
		log.Printf("a transmissão anunciada não abriu em %s, faixas de mídia fora do túnel", streamPendingTimeout)
	}
	w.mu.Unlock()
	if changed {
		select {
		case w.changed <- struct{}{}:
		default:
		}
	}
}

// Os endereços que aparecem aqui são dos servidores do Discord. A linha que traz o IP da
// pessoa ("Connected with local address") nem chega a ser lida como evento.
func (w *mediaWatch) handle(ev rtcEvent, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	waiting := w.split.waiting()
	changed := w.split.handle(ev, now)
	if !ev.Stream || !changed {
		return changed
	}
	switch {
	case ev.Kind == rtcCreating:
		log.Printf("transmissão abrindo no servidor %s, que entra no túnel", ev.Addr)
	case ev.Kind == rtcDestroy:
		log.Printf("transmissão encerrada")
	case !waiting && w.split.waiting():
		log.Printf("Go Live anunciado, faixas de mídia no túnel até o socket abrir")
	}
	return changed
}
