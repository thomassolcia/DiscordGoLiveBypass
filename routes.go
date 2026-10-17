package main

import (
	"context"
	"net"
	"net/netip"
	"time"
)

// Os dois jeitos de dividir o Discord entre o túnel e a conexão normal.
const (
	// Só a transmissão sai pelo IP de fora. Voz, mensagens, anexos e o teste de latência
	// saem direto, e a chamada fica num servidor do Brasil.
	modeStream = "transmissao"
	// O Discord inteiro no túnel, como até a 2.1. A câmera só funciona assim, mas a voz
	// passa a ir até o servidor da VPN e voltar.
	modeFull = "completo"
)

func validMode(mode string) bool { return mode == modeStream || mode == modeFull }

// Instalação de antes da 2.2 não tem modo guardado e passa a dividir.
func effectiveMode(mode string) string {
	if validMode(mode) {
		return mode
	}
	return modeStream
}

// Conferido em outubro de 2026 no registro do RIPE (objetos do mantenedor
// lir-us-discord1-1-MNT) e no DNS dos domínios do Discord. Voz e vídeo saíam dos blocos
// próprios dele, um por região, e é justamente esse tráfego UDP que o proxy da 1.x não
// alcançava. API, gateway e CDN ficam num trecho da Cloudflare reservado a ele.
//
// Desde setembro os servidores de voz (os c-gru, c-ord, c-iad... do discord.media) estão na
// Cloudflare, em 104.29.128.0/18, que ela anuncia em blocos /24 separados e que não aparece
// em nenhum registro com o nome do Discord. Quem mostra é o próprio cliente: o log dele diz
// a que IP a mídia conectou, e a lista em latency.discord.media/rtc só tem endereços dali.
var discordPrefixes = []netip.Prefix{
	netip.MustParsePrefix("66.22.192.0/18"),
	netip.MustParsePrefix("195.62.89.0/24"),
	netip.MustParsePrefix("162.159.128.0/20"),
	netip.MustParsePrefix("104.29.128.0/18"),
	netip.MustParsePrefix("2a0e:5940::/29"),
	netip.MustParsePrefix("2606:4700:7::a29f:8000/116"),
}

// Os servidores de mídia, onde ficam as conexões UDP de voz, câmera e transmissão. No modo
// transmissão eles saem direto, e só o servidor de cada transmissão entra no túnel.
var mediaPrefixes = []netip.Prefix{
	netip.MustParsePrefix("66.22.192.0/18"),
	netip.MustParsePrefix("195.62.89.0/24"),
	netip.MustParsePrefix("104.29.128.0/18"),
	netip.MustParsePrefix("2a0e:5940::/29"),
}

// Se o Discord mudar algum domínio de lugar, o endereço novo ainda entra no túnel pela
// resolução feita na hora de abrir, e a lista fixa vira só a base.
var discordHosts = []string{
	"discord.com",
	"gateway.discord.gg",
	"discord.gg",
	"discordapp.com",
	"cdn.discordapp.com",
	"media.discordapp.net",
	"images-ext-1.discordapp.net",
	"discord.media",
	"latency.discord.media",
	"updates.discord.com",
}

// O que pode sair direto no modo transmissão. É no gateway e na API que o Discord decide se
// o Go Live aparece, e eles ficam no túnel; o resto aqui é conteúdo e controle de voz.
//
// O teste de latência é o que escolhe a região da chamada: a lista que latency.discord.media
// devolve depende do IP de quem pergunta, e pelo túnel só vinham regiões dos EUA. Os
// c-gru..., c-ord... que coordenam cada conexão de voz respondem nos mesmos endereços de
// discord.media. O discordapp.com divide os endereços com a CDN e hoje só redireciona.
var directHosts = map[string]bool{
	"discordapp.com":              true,
	"cdn.discordapp.com":          true,
	"media.discordapp.net":        true,
	"images-ext-1.discordapp.net": true,
	"discord.media":               true,
	"latency.discord.media":       true,
}

func covered(prefixes []netip.Prefix, addr netip.Addr) bool {
	addr = addr.Unmap()
	for _, prefix := range prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func routable(addr netip.Addr) bool {
	return addr.IsValid() && !addr.IsLoopback() && !addr.IsPrivate() && !addr.IsUnspecified() &&
		!addr.IsLinkLocalUnicast() && !addr.IsMulticast()
}

func hostRoute(addr netip.Addr) netip.Prefix {
	addr = addr.Unmap()
	return netip.PrefixFrom(addr, addr.BitLen())
}

func TunnelRoutes(resolved []netip.Addr) []netip.Prefix {
	routes := append([]netip.Prefix(nil), discordPrefixes...)
	for _, addr := range resolved {
		addr = addr.Unmap()
		if !routable(addr) || covered(routes, addr) {
			continue
		}
		routes = append(routes, hostRoute(addr))
	}
	return routes
}

// Os endereços de cada host, como saíram do DNS.
type resolvedHosts map[string][]netip.Addr

func (r resolvedHosts) all() []netip.Addr {
	var addrs []netip.Addr
	for _, host := range discordHosts {
		addrs = append(addrs, r[host]...)
	}
	return addrs
}

// Um endereço que algum host do túnel também usa nunca sai direto: a Cloudflare escolhe o
// serviço pelo nome pedido, não pelo IP, e o gateway precisa do IP de fora.
func (r resolvedHosts) split() (tunnel, direct []netip.Addr) {
	needed := map[netip.Addr]bool{}
	for _, host := range discordHosts {
		if directHosts[host] {
			continue
		}
		for _, addr := range r[host] {
			needed[addr.Unmap()] = true
			tunnel = append(tunnel, addr.Unmap())
		}
	}
	for _, host := range discordHosts {
		if !directHosts[host] {
			continue
		}
		for _, addr := range r[host] {
			if addr = addr.Unmap(); !needed[addr] {
				needed[addr] = true
				direct = append(direct, addr)
			}
		}
	}
	return tunnel, direct
}

// StreamRoutes são as rotas fixas do modo transmissão: o trecho da Cloudflare do Discord
// sem os endereços que podem sair direto, e nenhum servidor de mídia. Os servidores das
// transmissões entram depois, um a um, pelo que o log do Discord mostra.
func StreamRoutes(resolved resolvedHosts) []netip.Prefix {
	var routes []netip.Prefix
	for _, prefix := range discordPrefixes {
		if !contains(mediaPrefixes, prefix) {
			routes = append(routes, prefix)
		}
	}
	tunnel, direct := resolved.split()
	for _, addr := range tunnel {
		if routable(addr) && !covered(routes, addr) {
			routes = append(routes, hostRoute(addr))
		}
	}
	return exclude(routes, direct)
}

func routesFor(mode string, resolved resolvedHosts) []netip.Prefix {
	if mode == modeFull {
		return TunnelRoutes(resolved.all())
	}
	return StreamRoutes(resolved)
}

// exclude devolve faixas que cobrem o mesmo que prefixes, menos os endereços de addrs. A
// faixa que contém algum deles se parte ao meio até sobrar só o que fica de fora, o que dá
// no máximo uma rota a mais por bit de diferença entre a faixa e o endereço.
func exclude(prefixes []netip.Prefix, addrs []netip.Addr) []netip.Prefix {
	var out []netip.Prefix
	for _, prefix := range prefixes {
		out = carve(prefix.Masked(), addrs, out)
	}
	return out
}

func carve(prefix netip.Prefix, addrs []netip.Addr, out []netip.Prefix) []netip.Prefix {
	if !anyInside(prefix, addrs) {
		return append(out, prefix)
	}
	if prefix.IsSingleIP() {
		return out
	}
	low, high := halves(prefix)
	return carve(high, addrs, carve(low, addrs, out))
}

func anyInside(prefix netip.Prefix, addrs []netip.Addr) bool {
	for _, addr := range addrs {
		if prefix.Contains(addr.Unmap()) {
			return true
		}
	}
	return false
}

func halves(prefix netip.Prefix) (netip.Prefix, netip.Prefix) {
	bits := prefix.Bits()
	raw := prefix.Addr().AsSlice()
	raw[bits/8] |= 0x80 >> (bits % 8)
	high, _ := netip.AddrFromSlice(raw)
	return netip.PrefixFrom(prefix.Addr(), bits+1), netip.PrefixFrom(high, bits+1)
}

// diffRoutes diz o que falta pôr e o que sobra tirar para a tabela aplicada virar a pedida,
// em ordem, para o log sair igual de uma vez para outra.
func diffRoutes(applied, want map[netip.Prefix]bool) (add, del []netip.Prefix) {
	missing, extra := map[netip.Prefix]bool{}, map[netip.Prefix]bool{}
	for prefix := range want {
		if !applied[prefix] {
			missing[prefix] = true
		}
	}
	for prefix := range applied {
		if !want[prefix] {
			extra[prefix] = true
		}
	}
	return sortedPrefixes(missing), sortedPrefixes(extra)
}

func contains[T comparable](list []T, item T) bool {
	for _, x := range list {
		if x == item {
			return true
		}
	}
	return false
}

// Falha de DNS aqui não impede nada: a lista fixa já cobre o que o Discord usa hoje.
func ResolveDiscordHosts(timeout time.Duration) resolvedHosts {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	found := resolvedHosts{}
	for _, host := range discordHosts {
		if addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host); err == nil {
			found[host] = addrs
		}
	}
	return found
}
