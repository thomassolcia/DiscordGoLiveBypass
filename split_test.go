package main

import (
	"net/netip"
	"testing"
	"time"
)

// Linhas de verdade do renderer_js.log do Discord Canary, de outubro de 2026. Ficaram de
// fora as "Connected with local address", que trazem o IP da pessoa.
const (
	linhaInicio        = `[2026-10-13 23:36:40.707] [info]  [startStreamWithSource] Starting stream for source id window:1574536 and name VALORANT   and pid 16680`
	linhaConectando    = `[2026-10-13 23:36:41.966] [info]  [RTCConnection(1559756752117833818, stream)] Connecting to RTC server wss://c-iad15-76632a45.discord.media:2083/, rtc-connection-id: 0f2f30cd-b1f1-4dc5-b917-1e2f94159b4a, channel: 1559756733310828`
	linhaCriando       = `[2026-10-13 23:36:43.075] [info]  [Connection(stream)] Creating connection to 104.29.152.78:19309 with audio ssrc: 7289`
	linhaConectado     = `[2026-10-13 23:36:43.214] [info]  [RTCConnection(1559756752117833818, stream)] RTC connected to media server: 104.29.152.78:19309`
	linhaDesconectado  = `[2026-10-13 23:37:25.557] [info]  [RTCConnection(1559756752117833818, stream)] Disconnected from RTC server, clean: true, code: 1000, reason: Force Close, state: RTC_CONNECTED`
	linhaFim           = `[2026-10-13 23:37:25.557] [info]  [RTCConnection(1559756752117833818, stream)] Destroy RTCConnection`
	linhaVozConectando = `[2026-10-17 00:40:01.100] [info]  [RTCConnection(713158092061278278, default)] Connecting to RTC server wss://c-gru08-0a1b2c3d.discord.media:2096/, rtc-connection-id: 5ddf3b6c-0023-4622-b16c-6b948c473, channel: 713158092061278`
	linhaVozCriando    = `[2026-10-17 00:40:01.900] [info]  [Connection(default)] Creating connection to 104.29.136.227:50002 with audio ssrc: 1787`
	linhaVozConectado  = `[2026-10-17 00:40:02.050] [info]  [RTCConnection(713158092061278278, default)] RTC connected to media server: 104.29.136.227:50002`
	linhaVozFim        = `[2026-10-17 01:10:00.000] [info]  [RTCConnection(713158092061278278, default)] Destroy RTCConnection`
)

const idStream = "1559756752117833818"

var (
	servidorStream = netip.MustParseAddr("104.29.152.78")
	servidorVoz    = netip.MustParseAddr("104.29.136.227")
)

func TestParseRTCLineReconheceAsLinhasDoDiscord(t *testing.T) {
	for name, caso := range map[string]struct {
		linha string
		quer  rtcEvent
	}{
		"transmissão começando":   {linhaInicio, rtcEvent{Kind: rtcStreamStart, Stream: true}},
		"controle da transmissão": {linhaConectando, rtcEvent{Kind: rtcConnecting, Stream: true, ID: idStream}},
		"socket da transmissão":   {linhaCriando, rtcEvent{Kind: rtcCreating, Stream: true, Addr: servidorStream}},
		"mídia conectada":         {linhaConectado, rtcEvent{Kind: rtcConnected, Stream: true, ID: idStream, Addr: servidorStream}},
		"fim da transmissão":      {linhaFim, rtcEvent{Kind: rtcDestroy, Stream: true, ID: idStream}},
		"controle da voz":         {linhaVozConectando, rtcEvent{Kind: rtcConnecting, ID: "713158092061278278"}},
		"socket da voz":           {linhaVozCriando, rtcEvent{Kind: rtcCreating, Addr: servidorVoz}},
		"ipv6 com colchetes": {`[info]  [Connection(stream)] Creating connection to [2a0e:5940:1::1]:50001 with audio ssrc: 1`,
			rtcEvent{Kind: rtcCreating, Stream: true, Addr: netip.MustParseAddr("2a0e:5940:1::1")}},
		"ipv6 sem colchetes": {`[info]  [Connection(stream)] Creating connection to 2a0e:5940:1::1:50001 with audio ssrc: 1`,
			rtcEvent{Kind: rtcCreating, Stream: true, Addr: netip.MustParseAddr("2a0e:5940:1::1")}},
	} {
		ev, ok := parseRTCLine(caso.linha)
		if !ok || ev != caso.quer {
			t.Errorf("%s: parseRTCLine = %+v, %v; queria %+v", name, ev, ok, caso.quer)
		}
	}
}

func TestParseRTCLineIgnoraORestoDoLog(t *testing.T) {
	for _, linha := range []string{
		linhaDesconectado,
		`[2026-10-13 23:36:43.210] [info]  [Connection(stream)] Connected with local address 203.0.113.7:56608 and protocol: udp`,
		`[2026-10-17 00:46:02.294] [info]  [RTCConnection(713158092061278278, default)] Remote media sink wants: {"1787":0,"any":0}`,
		`[2026-10-13 23:36:41.967] [info]  [RTCConnection(1559756752117833818, stream)] RTC connection state: AWAITING_ENDPOINT => CONNECTING`,
		`[2026-10-13 23:36:43.213] [info]  [Connection(stream)] Connection state change: CONNECTING => CONNECTED`,
		`[info]  [Connection(stream)] Creating connection to lixo with audio ssrc: 1`,
		``,
	} {
		if ev, ok := parseRTCLine(linha); ok {
			t.Errorf("parseRTCLine(%q) = %+v, queria nada", linha, ev)
		}
	}
}

// Alimenta o splitter linha por linha, meio segundo entre uma e outra.
func alimentar(t *testing.T, s *splitter, inicio time.Time, linhas ...string) time.Time {
	t.Helper()
	agora := inicio
	for _, linha := range linhas {
		ev, ok := parseRTCLine(linha)
		if !ok {
			t.Fatalf("linha nao reconhecida: %s", linha)
		}
		s.handle(ev, agora)
		agora = agora.Add(500 * time.Millisecond)
	}
	return agora
}

func TestSplitterTransmissaoCompleta(t *testing.T) {
	s := newSplitter()
	agora := alimentar(t, s, time.Now(), linhaVozConectando, linhaVozCriando, linhaVozConectado)
	if rotas := s.routes(); len(rotas) != 0 {
		t.Fatalf("so com voz, rotas = %v, queria nenhuma", rotas)
	}

	// Entre o anúncio e o socket, a faixa de mídia inteira vai para o túnel, menos a voz.
	agora = alimentar(t, s, agora, linhaInicio, linhaConectando)
	rotas := s.routes()
	if !covered(rotas, servidorStream) || !covered(rotas, netip.MustParseAddr("66.22.200.10")) {
		t.Errorf("esperando o socket, rotas = %v, queria as faixas de midia", rotas)
	}
	if covered(rotas, servidorVoz) {
		t.Errorf("a voz em %s entrou no tunel junto com a transmissao", servidorVoz)
	}

	// Com o socket aberto, sobra só o servidor da transmissão.
	agora = alimentar(t, s, agora, linhaCriando, linhaConectado)
	if rotas := s.routes(); len(rotas) != 1 || rotas[0] != netip.MustParsePrefix("104.29.152.78/32") {
		t.Errorf("transmissao aberta, rotas = %v, queria so 104.29.152.78/32", rotas)
	}

	// Controle caindo não tira a transmissão do túnel; o fim dela, sim.
	if ev, ok := parseRTCLine(linhaDesconectado); ok {
		s.handle(ev, agora)
	}
	if len(s.routes()) != 1 {
		t.Error("o Disconnected tirou a transmissao do tunel")
	}
	alimentar(t, s, agora, linhaFim)
	if rotas := s.routes(); len(rotas) != 0 {
		t.Errorf("depois do fim, rotas = %v, queria nenhuma", rotas)
	}
}

// Quem só assiste não passa pelo startStreamWithSource, mas abre a mesma conexão "stream".
func TestSplitterQuemAssisteTambemEntra(t *testing.T) {
	s := newSplitter()
	alimentar(t, s, time.Now(), linhaConectando, linhaCriando)
	if !covered(s.routes(), servidorStream) {
		t.Errorf("rotas = %v, queria o servidor da transmissao assistida", s.routes())
	}
}

func TestSplitterDesisteDaTransmissaoQueNaoAbriu(t *testing.T) {
	s := newSplitter()
	inicio := time.Now()
	alimentar(t, s, inicio, linhaInicio)

	if s.expire(inicio.Add(9 * time.Second)) {
		t.Error("desistiu antes do prazo")
	}
	if !s.expire(inicio.Add(11 * time.Second)) {
		t.Fatal("nao desistiu depois do prazo")
	}
	if rotas := s.routes(); len(rotas) != 0 {
		t.Errorf("depois de desistir, rotas = %v, queria nenhuma", rotas)
	}
	if s.expire(inicio.Add(12 * time.Second)) {
		t.Error("expire avisou mudanca sem nada esperando")
	}
}

// Se a conexão for destruída antes do socket, a faixa de mídia sai na hora.
func TestSplitterTransmissaoCanceladaAntesDoSocket(t *testing.T) {
	s := newSplitter()
	alimentar(t, s, time.Now(), linhaConectando, linhaFim)
	if rotas := s.routes(); len(rotas) != 0 {
		t.Errorf("rotas = %v, queria nenhuma", rotas)
	}
}

// Transmitir e assistir no mesmo servidor de mídia: fechar uma não tira a outra.
func TestSplitterDuasTransmissoesNoMesmoServidor(t *testing.T) {
	s := newSplitter()
	agora := time.Now()
	for _, ev := range []rtcEvent{
		{Kind: rtcConnecting, Stream: true, ID: "1"},
		{Kind: rtcCreating, Stream: true, Addr: servidorStream},
		{Kind: rtcConnecting, Stream: true, ID: "2"},
		{Kind: rtcCreating, Stream: true, Addr: servidorStream},
		{Kind: rtcDestroy, Stream: true, ID: "1"},
	} {
		s.handle(ev, agora)
	}
	if !covered(s.routes(), servidorStream) {
		t.Error("fechar uma transmissao tirou a outra do tunel")
	}
	s.handle(rtcEvent{Kind: rtcDestroy, Stream: true, ID: "2"}, agora)
	if len(s.routes()) != 0 {
		t.Errorf("rotas = %v depois de fechar as duas", s.routes())
	}
}

// Serviço que começou a ler o log no meio da conexão: o socket aparece sem anúncio, e o id
// chega depois, na linha do servidor conectado.
func TestSplitterSocketSemAnuncio(t *testing.T) {
	s := newSplitter()
	agora := alimentar(t, s, time.Now(), linhaCriando)
	if !covered(s.routes(), servidorStream) {
		t.Fatal("socket sem anuncio ficou fora do tunel")
	}
	alimentar(t, s, agora, linhaConectado, linhaFim)
	if rotas := s.routes(); len(rotas) != 0 {
		t.Errorf("rotas = %v, queria nenhuma depois do fim", rotas)
	}
}

func TestSplitterSoChamadaDeVozNaoMexeEmNada(t *testing.T) {
	s := newSplitter()
	agora := time.Now()
	for _, linha := range []string{linhaVozConectando, linhaVozCriando, linhaVozConectado, linhaVozFim} {
		ev, _ := parseRTCLine(linha)
		if s.handle(ev, agora) {
			t.Errorf("%s mudou as rotas", linha)
		}
	}
}

func TestSplitterIgnoraEnderecoLocal(t *testing.T) {
	s := newSplitter()
	s.handle(rtcEvent{Kind: rtcCreating, Stream: true, Addr: netip.MustParseAddr("10.2.0.1")}, time.Now())
	s.handle(rtcEvent{Kind: rtcConnected, Stream: true, ID: "1", Addr: netip.MustParseAddr("127.0.0.1")}, time.Now())
	if rotas := s.routes(); len(rotas) != 0 {
		t.Errorf("rotas = %v, queria nenhuma", rotas)
	}
}
