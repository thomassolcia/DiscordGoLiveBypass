package main

import (
	"net/netip"
	"testing"
)

// Endereços reais do Discord em outubro de 2026, um de cada tipo de serviço.
func TestDiscordPrefixesCobremOQueODiscordUsa(t *testing.T) {
	for name, raw := range map[string]string{
		"api discord.com":    "162.159.128.233",
		"gateway":            "162.159.130.234",
		"discord.media":      "162.159.137.234",
		"voz no Brasil":      "66.22.200.10",
		"voz em Frankfurt":   "66.22.243.5",
		"discord.media ipv6": "2606:4700:7::a29f:89ea",
		"voz em ipv6":        "2a0e:5940:1::1",
		"ipv4 mapeado em v6": "::ffff:66.22.196.1",
	} {
		if !covered(discordPrefixes, netip.MustParseAddr(raw)) {
			t.Errorf("%s (%s) ficou fora do tunel", name, raw)
		}
	}
}

// Servidores de mídia que o Discord usou de verdade em outubro de 2026, tirados do log do
// cliente (as linhas "Creating connection to") e da lista em latency.discord.media/rtc. Foi
// por eles ficarem fora do túnel que a transmissão abria e quem assistia ficava carregando:
// a voz passava, mas o vídeo saía pelo IP brasileiro.
func TestDiscordPrefixesCobremAVozNaCloudflare(t *testing.T) {
	for name, raw := range map[string]string{
		"Go Live em São Paulo, c-gru20": "104.29.143.243",
		"voz em São Paulo, c-gru08":     "104.29.136.227",
		"voz em Chicago, c-ord12":       "104.29.158.178",
		"voz em Virgínia, c-iad09":      "104.29.159.170",
		"latência us-south":             "104.29.129.247",
	} {
		if !covered(discordPrefixes, netip.MustParseAddr(raw)) {
			t.Errorf("%s (%s) ficou fora do tunel", name, raw)
		}
	}
}

// O resto do computador não pode entrar no túnel: nem outro site da Cloudflare, nem o
// servidor de jogo que divide o provedor com o Discord.
func TestDiscordPrefixesNaoPegamOResto(t *testing.T) {
	for name, raw := range map[string]string{
		"1.1.1.1":                   "1.1.1.1",
		"cloudflare fora do trecho": "162.159.192.1",
		"outro site cloudflare":     "104.16.132.229",
		"warp da cloudflare":        "104.28.14.1",
		"cloudflare depois da voz":  "104.29.192.1",
		"servidor de jogo i3D":      "213.179.200.1",
		"google":                    "142.250.79.46",
	} {
		if covered(discordPrefixes, netip.MustParseAddr(raw)) {
			t.Errorf("%s (%s) entraria no tunel", name, raw)
		}
	}
}

func TestTunnelRoutesAcrescentaSoONovo(t *testing.T) {
	fixas := len(discordPrefixes)
	resolved := []netip.Addr{
		netip.MustParseAddr("162.159.135.232"), // já coberto
		netip.MustParseAddr("104.18.52.172"),   // fora da lista fixa
		netip.MustParseAddr("104.18.52.172"),   // repetido
		netip.MustParseAddr("127.0.0.1"),
		netip.MustParseAddr("10.2.0.1"),
	}

	routes := TunnelRoutes(resolved)
	if len(routes) != len(discordPrefixes)+1 {
		t.Fatalf("TunnelRoutes = %v, queria a lista fixa mais um", routes)
	}
	if last := routes[len(routes)-1]; last != netip.MustParsePrefix("104.18.52.172/32") {
		t.Errorf("rota acrescentada = %s, queria 104.18.52.172/32", last)
	}

	// A lista fixa não pode ser alterada por quem chamou.
	if len(discordPrefixes) != fixas {
		t.Errorf("TunnelRoutes mexeu na lista fixa: %v", discordPrefixes)
	}
}

// O que o DNS devolveu em 17 de outubro de 2026. Os conjuntos não se misturam: gateway, API,
// CDN, mídia dos anexos e discord.media usam cada um seus próprios endereços.
func dnsOutubro2026() resolvedHosts {
	addrs := func(raw ...string) []netip.Addr {
		var out []netip.Addr
		for _, r := range raw {
			out = append(out, netip.MustParseAddr(r))
		}
		return out
	}
	gateway := addrs("162.159.133.234", "162.159.135.234", "162.159.136.234", "162.159.134.234", "162.159.130.234")
	api := addrs("162.159.128.233", "162.159.135.232", "162.159.136.232", "162.159.137.232", "162.159.138.232")
	cdn := addrs("162.159.130.233", "162.159.133.233", "162.159.134.233", "162.159.135.233", "162.159.129.233")
	midia := addrs("162.159.128.232", "162.159.129.232", "162.159.134.232", "162.159.130.232", "162.159.133.232")
	rtc := addrs("162.159.128.235", "162.159.129.235", "162.159.130.235", "162.159.138.234", "162.159.137.234",
		"2606:4700:7::a29f:80eb", "2606:4700:7::a29f:82eb", "2606:4700:7::a29f:81eb", "2606:4700:7::a29f:8aea", "2606:4700:7::a29f:89ea")
	return resolvedHosts{
		"discord.com":                 api,
		"updates.discord.com":         api,
		"gateway.discord.gg":          gateway,
		"discord.gg":                  gateway,
		"discordapp.com":              cdn,
		"cdn.discordapp.com":          cdn,
		"media.discordapp.net":        midia,
		"images-ext-1.discordapp.net": midia,
		"discord.media":               rtc,
		"latency.discord.media":       rtc,
	}
}

func TestModoPadraoEATransmissao(t *testing.T) {
	for guardado, quer := range map[string]string{
		"":            modeStream,
		"transmissao": modeStream,
		"completo":    modeFull,
		"qualquer":    modeStream,
	} {
		if got := effectiveMode(guardado); got != quer {
			t.Errorf("effectiveMode(%q) = %q, queria %q", guardado, got, quer)
		}
	}
}

// Confere endereço por endereço que o recorte deixa de fora exatamente os pedidos, sem
// sobrepor faixas, para um bloco inteiro do Discord.
func TestExcludeTiraSoOsEnderecosPedidos(t *testing.T) {
	bloco := netip.MustParsePrefix("162.159.128.0/20")
	fora := []netip.Addr{
		netip.MustParseAddr("162.159.128.0"),
		netip.MustParseAddr("162.159.130.233"),
		netip.MustParseAddr("162.159.130.234"),
		netip.MustParseAddr("162.159.143.255"),
		netip.MustParseAddr("::ffff:162.159.137.234"),
		netip.MustParseAddr("104.29.143.243"), // de outra faixa, não muda nada
	}

	partes := exclude([]netip.Prefix{bloco}, fora)
	for addr := bloco.Addr(); bloco.Contains(addr); addr = addr.Next() {
		vezes := 0
		for _, parte := range partes {
			if parte.Contains(addr) {
				vezes++
			}
		}
		quer := 1
		if covered([]netip.Prefix{netip.MustParsePrefix("162.159.128.0/32"), netip.MustParsePrefix("162.159.130.233/32"),
			netip.MustParsePrefix("162.159.130.234/32"), netip.MustParsePrefix("162.159.143.255/32"),
			netip.MustParsePrefix("162.159.137.234/32")}, addr) {
			quer = 0
		}
		if vezes != quer {
			t.Fatalf("%s aparece em %d faixas do recorte, queria %d", addr, vezes, quer)
		}
	}
	if len(partes) > 5*12 {
		t.Errorf("recorte com %d faixas, mais que uma por bit de cada endereço", len(partes))
	}
}

func TestExcludeSemEnderecoDentroDevolveAFaixa(t *testing.T) {
	faixas := []netip.Prefix{netip.MustParsePrefix("66.22.192.0/18"), netip.MustParsePrefix("2a0e:5940::/29")}
	partes := exclude(faixas, []netip.Addr{netip.MustParseAddr("1.1.1.1")})
	if len(partes) != 2 || partes[0] != faixas[0] || partes[1] != faixas[1] {
		t.Errorf("exclude = %v, queria as faixas intactas", partes)
	}
}

func TestExcludeEmIPv6(t *testing.T) {
	bloco := netip.MustParsePrefix("2606:4700:7::a29f:8000/116")
	fora := netip.MustParseAddr("2606:4700:7::a29f:89ea")
	partes := exclude([]netip.Prefix{bloco}, []netip.Addr{fora})
	if covered(partes, fora) {
		t.Errorf("%s continuou no recorte %v", fora, partes)
	}
	for _, dentro := range []string{"2606:4700:7::a29f:8000", "2606:4700:7::a29f:89eb", "2606:4700:7::a29f:8fff"} {
		if !covered(partes, netip.MustParseAddr(dentro)) {
			t.Errorf("%s saiu do recorte junto", dentro)
		}
	}
	if len(partes) != 12 {
		t.Errorf("recorte de um /116 sem um endereço com %d faixas, queria 12", len(partes))
	}
}

// No modo transmissão o que decide o Go Live continua no túnel e todo o resto sai direto.
func TestStreamRoutesSeparaOQueSaiDireto(t *testing.T) {
	dns := dnsOutubro2026()
	rotas := StreamRoutes(dns)

	for _, host := range []string{"discord.com", "gateway.discord.gg", "discord.gg", "updates.discord.com"} {
		for _, addr := range dns[host] {
			if !covered(rotas, addr) {
				t.Errorf("%s (%s) saiu do tunel", host, addr)
			}
		}
	}
	for _, host := range []string{"cdn.discordapp.com", "media.discordapp.net", "latency.discord.media", "discord.media"} {
		for _, addr := range dns[host] {
			if covered(rotas, addr) {
				t.Errorf("%s (%s) continuou no tunel", host, addr)
			}
		}
	}
	for name, raw := range map[string]string{
		"voz em São Paulo, c-gru08": "104.29.136.227",
		"Go Live em São Paulo":      "104.29.143.243",
		"voz antiga no Brasil":      "66.22.200.10",
		"voz antiga em ipv6":        "2a0e:5940:1::1",
		"outro site cloudflare":     "104.16.132.229",
	} {
		if covered(rotas, netip.MustParseAddr(raw)) {
			t.Errorf("%s (%s) entraria no tunel fora de uma transmissao", name, raw)
		}
	}
	// O trecho da Cloudflare que não aparece em nenhum host continua no túnel: se o
	// Discord puser o gateway num endereço novo, ele não escapa pelo IP brasileiro.
	if !covered(rotas, netip.MustParseAddr("162.159.131.10")) {
		t.Error("endereco desconhecido do trecho do Discord saiu do tunel")
	}
	if len(rotas) > 400 {
		t.Errorf("modo transmissao com %d rotas, recorte grande demais", len(rotas))
	}
}

// Se um dia a CDN e o gateway dividirem endereço, o gateway ganha: perder o Go Live é pior
// que baixar um anexo pela VPN.
func TestStreamRoutesNuncaTiraOGateway(t *testing.T) {
	dns := dnsOutubro2026()
	dividido := netip.MustParseAddr("162.159.135.234")
	dns["cdn.discordapp.com"] = append(dns["cdn.discordapp.com"], dividido)

	if !covered(StreamRoutes(dns), dividido) {
		t.Errorf("%s e do gateway e saiu do tunel por tambem ser da CDN", dividido)
	}
}

func TestStreamRoutesSemDNSMantemOTrechoInteiro(t *testing.T) {
	rotas := StreamRoutes(resolvedHosts{})
	for _, raw := range []string{"162.159.128.0", "162.159.130.234", "162.159.143.255", "2606:4700:7::a29f:8001"} {
		if !covered(rotas, netip.MustParseAddr(raw)) {
			t.Errorf("%s ficou fora do tunel sem DNS", raw)
		}
	}
	if covered(rotas, netip.MustParseAddr("104.29.136.227")) {
		t.Error("servidor de voz entrou no tunel no modo transmissao")
	}
}

// Gateway que mudasse para a faixa de mídia entra sozinho, com um /32.
func TestStreamRoutesSegueOGatewayParaForaDoTrecho(t *testing.T) {
	novo := netip.MustParseAddr("104.29.140.1")
	rotas := StreamRoutes(resolvedHosts{"gateway.discord.gg": {novo}})
	if !covered(rotas, novo) || covered(rotas, netip.MustParseAddr("104.29.140.2")) {
		t.Errorf("rotas = %v, queria so %s/32 da faixa de midia", rotas, novo)
	}
}

func TestModoCompletoEOMesmoDaVersaoAnterior(t *testing.T) {
	dns := dnsOutubro2026()
	rotas := routesFor(modeFull, dns)
	for _, raw := range []string{"162.159.129.233", "162.159.137.234", "104.29.136.227", "66.22.200.10"} {
		if !covered(rotas, netip.MustParseAddr(raw)) {
			t.Errorf("%s ficou fora do tunel no modo completo", raw)
		}
	}
	if len(rotas) != len(discordPrefixes) {
		t.Errorf("modo completo com %d rotas, queria so a lista fixa (%d)", len(rotas), len(discordPrefixes))
	}
}
