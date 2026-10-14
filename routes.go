package main

import (
	"context"
	"net"
	"net/netip"
	"time"
)

// Conferido em outubro de 2026 no registro do RIPE (objetos do mantenedor
// lir-us-discord1-1-MNT) e no DNS dos domínios do Discord. Voz e vídeo saem dos blocos
// próprios dele, um por região, e é justamente esse tráfego UDP que o proxy da 1.x não
// alcançava. API, gateway e CDN ficam num trecho da Cloudflare reservado a ele.
var discordPrefixes = []netip.Prefix{
	netip.MustParsePrefix("66.22.192.0/18"),
	netip.MustParsePrefix("195.62.89.0/24"),
	netip.MustParsePrefix("162.159.128.0/20"),
	netip.MustParsePrefix("2a0e:5940::/29"),
	netip.MustParsePrefix("2606:4700:7::a29f:8000/116"),
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
	"discord.media",
	"latency.discord.media",
	"updates.discord.com",
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

func TunnelRoutes(resolved []netip.Addr) []netip.Prefix {
	routes := append([]netip.Prefix(nil), discordPrefixes...)
	for _, addr := range resolved {
		addr = addr.Unmap()
		if !addr.IsValid() || addr.IsLoopback() || addr.IsPrivate() || addr.IsUnspecified() || covered(routes, addr) {
			continue
		}
		routes = append(routes, netip.PrefixFrom(addr, addr.BitLen()))
	}
	return routes
}

// Falha de DNS aqui não impede nada: a lista fixa já cobre o que o Discord usa hoje.
func ResolveDiscordHosts(timeout time.Duration) []netip.Addr {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var found []netip.Addr
	for _, host := range discordHosts {
		addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			continue
		}
		found = append(found, addrs...)
	}
	return found
}
