package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// Mídia e anexo saem por fora: mandar isso por proxy público gratuito transforma qualquer
// imagem em dez segundos de espera, e nada disso muda a região que o Discord enxerga.
const defaultBypass = "cdn.discordapp.com;*.discordapp.net;*.discord.media;<local>"

type options struct {
	manual   string
	exclude  string
	channel  string
	exePath  string
	bypass   string
	force    bool
	noCache  bool
	noTor    bool
	checkRun bool
	fallback bool
	deadline time.Duration
	extra    []string
}

func main() {
	opts := options{}

	flag.StringVar(&opts.manual, "proxy", "", "proxy fixo no formato esquema://host:porta, pula a busca automatica")
	flag.StringVar(&opts.exclude, "exclude", "BR", "codigos de pais recusados na saida, separados por virgula")
	flag.StringVar(&opts.channel, "channel", "auto", "canal do Discord: auto, stable, ptb ou canary")
	flag.StringVar(&opts.exePath, "exe", "", "caminho explicito do executavel, ignora a deteccao automatica")
	flag.StringVar(&opts.bypass, "bypass", defaultBypass, "lista de dominios que saem por fora do proxy")
	flag.BoolVar(&opts.force, "force", false, "encerra um Discord ja aberto antes de subir o novo")
	flag.BoolVar(&opts.noCache, "no-cache", false, "ignora a proxy guardada da execucao anterior")
	flag.BoolVar(&opts.noTor, "no-tor", false, "nao procura um cliente Tor local")
	flag.BoolVar(&opts.checkRun, "check", false, "so procura e valida a proxy, nao abre o Discord")
	flag.BoolVar(&opts.fallback, "fallback", false, "permite conexao direta quando o proxy falhar: o Discord sobrevive, mas sai pelo seu IP sem avisar")
	flag.DurationVar(&opts.deadline, "timeout", 3*time.Minute, "prazo total para achar uma proxy valida")

	flag.Usage = usage
	flag.Parse()
	opts.extra = flag.Args()

	ui := NewUI()
	if err := run(opts, ui); err != nil {
		ui.Fatal(err)
		os.Exit(1)
	}
	ui.Close()
}

func usage() {
	fmt.Fprintf(os.Stderr, `DiscordGoLiveBypass: abre o Discord ja apontado para um IP fora do Brasil.

uso: %s [flags] [-- argumentos extras para o Discord]

`, os.Args[0])
	flag.PrintDefaults()
}

func run(opts options, ui *UI) error {
	excluded := ParseExcluded(opts.exclude)
	if len(excluded) == 0 {
		return errors.New("a lista de paises recusados ficou vazia, informe algo como -exclude BR")
	}

	steps := 4
	if opts.checkRun {
		steps = 3
	}
	ui.Plan(steps)
	ui.Banner()

	// Resolver o executável antes da busca evita gastar dois minutos procurando proxy para
	// só então descobrir que o Discord nem está instalado.
	ui.Step("Procurando a instalacao do Discord")
	channel, binary, err := resolveTarget(opts, ui)
	if err != nil {
		if !opts.checkRun {
			return err
		}
		ui.Warn("%v", err)
	}

	if binary != "" {
		if _, err := os.Stat(binary); err != nil {
			return fmt.Errorf("nao consegui usar o executavel %s: %w", binary, err)
		}
		ui.Ok("%s encontrado", channel.Friendly)
		ui.Detail("%s", binary)

		if running, err := IsRunning(channel); err != nil {
			ui.Warn("nao consegui checar se o %s ja esta aberto: %v", channel.Friendly, err)
		} else if running {
			switch {
			case opts.checkRun:
				ui.Warn("o %s esta aberto agora, uma abertura de verdade pediria -force", channel.Friendly)
			case !opts.force:
				return fmt.Errorf("o %s ja esta aberto. Ele e instancia unica, entao abrir de novo so acorda a janela antiga, que continua no IP daqui. Feche-o ou rode com -force", channel.Friendly)
			default:
				ui.Warn("o %s ja esta aberto, sera reiniciado no fim", channel.Friendly)
			}
		}
	}

	ui.Step("Procurando uma saida fora de %s", excludedLabel(excluded))
	endpoint, latency, err := resolveProxy(opts, excluded, ui)
	if err != nil {
		return err
	}

	ui.Step("Confirmando por onde a conexao vai sair")
	ui.Busy("consultando o servico de geolocalizacao por dentro do proxy")
	country, err := ExitCountry(endpoint, probeTimeout)
	if err != nil {
		return fmt.Errorf("nao consegui confirmar o pais de saida de %s: %w", endpoint, err)
	}
	ui.Ok("a saida esta em %s", CountryLabel(country))

	if err := StoreCachedProxy(endpoint); err != nil {
		ui.Warn("nao consegui guardar a proxy para a proxima vez: %v", err)
	} else {
		ui.Detail("guardada para acelerar a proxima execucao")
	}

	summary := [][2]string{
		{"saida", CountryLabel(country)},
		{"servidor", endpoint.String()},
		{"resposta", fmt.Sprintf("%d ms", latency.Milliseconds())},
	}

	if opts.checkRun {
		ui.Summary("proxy validada", append(summary, [2]string{"discord", "nao foi aberto, modo -check"}))
		return nil
	}

	// Só encerra a instância antiga agora, com um proxy vivo na mão. Matar antes deixaria a
	// pessoa sem Discord nenhum se a busca falhasse.
	ui.Step("Abrindo o %s", channel.Friendly)
	if err := terminateIfRunning(channel, ui); err != nil {
		return err
	}

	args := append(ProxyArgs(endpoint, opts.bypass, opts.fallback), opts.extra...)
	ui.Busy("subindo o processo com as flags de proxy")
	pid, err := Launch(binary, args)
	if err != nil {
		return fmt.Errorf("nao consegui abrir o %s: %w", channel.Friendly, err)
	}
	ui.Ok("%s aberto, pid %d", channel.Friendly, pid)
	ui.Detail("%s", args[0])

	ui.Summary("tudo pronto", append(summary,
		[2]string{"discord", fmt.Sprintf("%s, pid %d", channel.Friendly, pid)},
		[2]string{"sem proxy", opts.bypass}))
	return nil
}

func excludedLabel(excluded map[string]bool) string {
	codes := make([]string, 0, len(excluded))
	for code := range excluded {
		codes = append(codes, code)
	}
	sort.Strings(codes)

	names := make([]string, 0, len(codes))
	for _, code := range codes {
		names = append(names, CountryLabel(code))
	}
	return strings.Join(names, ", ")
}

func resolveTarget(opts options, ui *UI) (Channel, string, error) {
	if opts.channel != "auto" {
		channel, err := LookupChannel(opts.channel)
		if err != nil {
			return Channel{}, "", err
		}
		if opts.exePath != "" {
			return channel, opts.exePath, nil
		}

		ui.Busy("procurando o %s dentro de %%LOCALAPPDATA%%", channel.Friendly)
		binary, err := LocateDiscord(channel)
		return channel, binary, err
	}

	if opts.exePath != "" {
		return ChannelForBinary(opts.exePath), opts.exePath, nil
	}

	ui.Busy("procurando dentro de %%LOCALAPPDATA%%")
	channel, binary, err := DetectChannel()
	if err != nil {
		return Channel{}, "", err
	}
	return channel, binary, nil
}

func terminateIfRunning(channel Channel, ui *UI) error {
	running, err := IsRunning(channel)
	if err != nil || !running {
		return nil
	}

	ui.Busy("encerrando o %s que ja estava aberto", channel.Friendly)
	if err := Terminate(channel); err != nil {
		return err
	}
	ui.Ok("instancia antiga encerrada")

	// O Windows demora um pouco para soltar o lock de instância única. Subir em cima disso
	// faz o processo novo se achar duplicado e sair sozinho.
	ui.Busy("esperando o Windows soltar a trava de instancia unica")
	time.Sleep(2 * time.Second)
	return nil
}

// As fontes vêm da mais previsível para a mais lenta.
func resolveProxy(opts options, excluded map[string]bool, ui *UI) (Endpoint, time.Duration, error) {
	deadline := time.Now().Add(opts.deadline)

	if opts.manual != "" {
		endpoint, ok := ParseProxy(opts.manual)
		if !ok {
			return Endpoint{}, 0, fmt.Errorf("proxy invalido: %q. Use algo como socks5://1.2.3.4:1080", opts.manual)
		}

		ui.Busy("testando o proxy que voce passou: %s", endpoint)
		latency, err := Probe(endpoint, probeTimeout)
		if err != nil {
			// Quem escolheu um proxy quer aquele, então não caímos para a busca automática:
			// ser mandado para um endereço desconhecido em silêncio é pior que o erro.
			return Endpoint{}, 0, fmt.Errorf("seu proxy %s nao respondeu: %w", endpoint, err)
		}

		ui.Ok("seu proxy respondeu em %d ms", latency.Milliseconds())
		return endpoint, latency, nil
	}

	if !opts.noCache {
		if cached, ok := ReadCachedProxy(); ok {
			ui.Busy("revalidando a proxy da execucao anterior: %s", cached)
			if latency, err := Probe(cached, fastProbeTimeout); err == nil {
				ui.Ok("a proxy guardada ainda serve, %d ms", latency.Milliseconds())
				ui.Detail("%s", cached)
				return cached, latency, nil
			}
			ui.Warn("a proxy guardada nao respondeu mais, procurando outra")
		}
	}

	if !opts.noTor {
		ui.Busy("procurando um Tor local nas portas conhecidas")
		if tor, latency, ok := DetectTor(excluded, ui); ok {
			return tor, latency, nil
		}
	}

	return PickFreeProxy(excluded, deadline, ui)
}
