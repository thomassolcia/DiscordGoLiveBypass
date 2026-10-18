package main

import (
	"net/netip"
	"regexp"
	"sort"
	"strings"
	"time"
)

// O Discord abre duas conexões de mídia por chamada: a "default", que leva voz e câmera, e
// uma "stream" para cada Go Live, de quem transmite e de quem assiste. Só a segunda precisa
// do IP de fora. O endereço do servidor não dá para saber antes, mas o cliente escreve no
// renderer_js.log cada passo da conexão, e entre anunciar uma transmissão e abrir o socket
// dela passa quase meio segundo, tempo de sobra para a rota entrar.
type rtcKind int

const (
	// [startStreamWithSource] Starting stream for source ...
	rtcStreamStart rtcKind = iota + 1
	// [RTCConnection(id, stream)] Connecting to RTC server wss://...
	rtcConnecting
	// [Connection(stream)] Creating connection to 104.29.152.78:19309 with audio ssrc: 7289
	rtcCreating
	// [RTCConnection(id, stream)] RTC connected to media server: 104.29.152.78:19309
	rtcConnected
	// [RTCConnection(id, stream)] Destroy RTCConnection
	//
	// "Disconnected from RTC server" não serve como fim: aparece também quando o controle cai
	// e o cliente reconecta no mesmo servidor, com a mídia ainda passando.
	rtcDestroy
)

type rtcEvent struct {
	Kind   rtcKind
	Stream bool // conexão do Go Live, e não a de voz e câmera
	ID     string
	Addr   netip.Addr
}

var (
	rtcConnectionLine = regexp.MustCompile(`\[RTCConnection\((\d+), (default|stream)\)\] (Connecting to RTC server|RTC connected to media server: (\S+)|Destroy RTCConnection)`)
	mediaCreatingLine = regexp.MustCompile(`\[Connection\((default|stream)\)\] Creating connection to (\S+) `)
)

func parseRTCLine(line string) (rtcEvent, bool) {
	if !strings.Contains(line, "Connection") && !strings.Contains(line, "startStreamWithSource") {
		return rtcEvent{}, false
	}
	if strings.Contains(line, "[startStreamWithSource] Starting stream") {
		return rtcEvent{Kind: rtcStreamStart, Stream: true}, true
	}
	if m := mediaCreatingLine.FindStringSubmatch(line); m != nil {
		addr, ok := parseMediaAddr(m[2])
		return rtcEvent{Kind: rtcCreating, Stream: m[1] == "stream", Addr: addr}, ok
	}
	m := rtcConnectionLine.FindStringSubmatch(line)
	if m == nil {
		return rtcEvent{}, false
	}
	ev := rtcEvent{ID: m[1], Stream: m[2] == "stream"}
	switch {
	case strings.HasPrefix(m[3], "Connecting"):
		ev.Kind = rtcConnecting
	case strings.HasPrefix(m[3], "Destroy"):
		ev.Kind = rtcDestroy
	default:
		addr, ok := parseMediaAddr(m[4])
		if !ok {
			return rtcEvent{}, false
		}
		ev.Kind, ev.Addr = rtcConnected, addr
	}
	return ev, true
}

// O cliente escreve ip:porta, e um IPv6 talvez sem colchetes.
func parseMediaAddr(raw string) (netip.Addr, bool) {
	if addrPort, err := netip.ParseAddrPort(raw); err == nil {
		return addrPort.Addr().Unmap(), true
	}
	if i := strings.LastIndexByte(raw, ':'); i > 0 {
		if addr, err := netip.ParseAddr(strings.Trim(raw[:i], "[]")); err == nil {
			return addr.Unmap(), true
		}
	}
	return netip.Addr{}, false
}

// Se a transmissão anunciada não chegar a abrir o socket nesse prazo, as faixas de mídia
// saem do túnel e a voz não fica presa nele.
const streamPendingTimeout = 10 * time.Second

// O id de uma linha "Creating connection", que vem sem id, quando nenhuma conexão de
// transmissão foi anunciada antes, como no serviço que começou a ler o log no meio dela.
const unknownStream = "?"

// splitter guarda as conexões de mídia abertas e diz o que precisa estar no túnel além das
// rotas fixas do modo. Não toca em rota nenhuma: quem aplica é o motor.
type splitter struct {
	pending   time.Time // transmissão anunciada esperando o socket
	pendingID string
	lastVoice string
	streams   map[string]netip.Addr // conexão de transmissão -> servidor de mídia
	voice     map[string]netip.Addr // conexão de voz -> servidor de mídia
}

func newSplitter() *splitter {
	return &splitter{streams: map[string]netip.Addr{}, voice: map[string]netip.Addr{}}
}

// handle aplica um evento e diz se as rotas que o splitter pede mudaram.
func (s *splitter) handle(ev rtcEvent, now time.Time) bool {
	before := s.routesKey()
	switch ev.Kind {
	case rtcStreamStart:
		s.pending = now
	case rtcConnecting:
		if ev.Stream {
			s.pending, s.pendingID = now, ev.ID
		} else {
			s.lastVoice = ev.ID
		}
	case rtcCreating:
		if !routable(ev.Addr) {
			break
		}
		if ev.Stream {
			id := s.pendingID
			if id == "" {
				id = unknownStream
			}
			s.streams[id] = ev.Addr
			s.pending, s.pendingID = time.Time{}, ""
		} else if s.lastVoice != "" {
			s.voice[s.lastVoice] = ev.Addr
		}
	case rtcConnected:
		if !routable(ev.Addr) {
			break
		}
		if ev.Stream {
			if s.streams[unknownStream] == ev.Addr {
				delete(s.streams, unknownStream)
			}
			s.streams[ev.ID] = ev.Addr
		} else {
			s.voice[ev.ID] = ev.Addr
		}
	case rtcDestroy:
		if ev.Stream {
			delete(s.streams, ev.ID)
			if s.pendingID == ev.ID {
				s.pending, s.pendingID = time.Time{}, ""
			}
		} else {
			delete(s.voice, ev.ID)
		}
	}
	return s.routesKey() != before
}

// expire encerra a espera de uma transmissão que não abriu e diz se isso mudou as rotas.
func (s *splitter) expire(now time.Time) bool {
	if s.pending.IsZero() || now.Sub(s.pending) < streamPendingTimeout {
		return false
	}
	s.pending, s.pendingID = time.Time{}, ""
	return true
}

func (s *splitter) waiting() bool { return !s.pending.IsZero() }

// streamAddrs são os servidores das transmissões abertas, sem repetição e em ordem.
func (s *splitter) streamAddrs() []netip.Addr {
	seen := map[netip.Addr]bool{}
	var addrs []netip.Addr
	for _, addr := range s.streams {
		if !seen[addr] {
			seen[addr] = true
			addrs = append(addrs, addr)
		}
	}
	sort.Slice(addrs, func(i, j int) bool { return addrs[i].Less(addrs[j]) })
	return addrs
}

// routes é o que precisa estar no túnel agora. Enquanto uma transmissão espera o socket,
// entram as faixas de mídia inteiras, menos os servidores de voz em uso: a voz não muda de
// caminho no meio da chamada só porque alguém abriu um Go Live.
func (s *splitter) routes() []netip.Prefix {
	set := map[netip.Prefix]bool{}
	for _, addr := range s.streamAddrs() {
		set[hostRoute(addr)] = true
	}
	if s.waiting() {
		var voice []netip.Addr
		for _, addr := range s.voice {
			voice = append(voice, addr)
		}
		for _, prefix := range exclude(mediaPrefixes, voice) {
			set[prefix] = true
		}
	}
	return sortedPrefixes(set)
}

func (s *splitter) routesKey() string {
	var b strings.Builder
	for _, prefix := range s.routes() {
		b.WriteString(prefix.String())
		b.WriteByte(' ')
	}
	return b.String()
}

func sortedPrefixes(set map[netip.Prefix]bool) []netip.Prefix {
	list := make([]netip.Prefix, 0, len(set))
	for prefix := range set {
		list = append(list, prefix)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Addr() != list[j].Addr() {
			return list[i].Addr().Less(list[j].Addr())
		}
		return list[i].Bits() < list[j].Bits()
	})
	return list
}
