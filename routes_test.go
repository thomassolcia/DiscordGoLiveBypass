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
