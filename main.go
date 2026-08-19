package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
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

	if err := run(opts); err != nil {
		fmt.Fprintf(os.Stderr, "\nerro: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `DiscordGoLiveBypass: abre o Discord ja apontado para um IP fora do Brasil.

uso: %s [flags] [-- argumentos extras para o Discord]

`, os.Args[0])
	flag.PrintDefaults()
}

func logf(format string, args ...any) {
	fmt.Printf("%s %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
}

func run(opts options) error {
	excluded := ParseExcluded(opts.exclude)
	if len(excluded) == 0 {
		return errors.New("a lista de paises recusados ficou vazia, informe algo como -exclude BR")
	}

	// Resolver o executável antes da busca evita gastar dois minutos procurando proxy para
	// só então descobrir que o Discord nem está instalado.
	channel, binary, err := resolveTarget(opts)
	if err != nil {
		if !opts.checkRun {
			return err
		}
		logf("aviso: %v", err)
	}

	if binary != "" {
		if _, err := os.Stat(binary); err != nil {
			return fmt.Errorf("nao consegui usar o executavel %s: %w", binary, err)
		}
		logf("executavel: %s", binary)

		if running, err := IsRunning(channel); err != nil {
			logf("aviso: nao consegui checar se o %s ja esta aberto: %v", channel.Friendly, err)
		} else if running {
			if opts.checkRun {
				logf("aviso: o %s esta aberto agora, uma abertura de verdade pediria -force", channel.Friendly)
			} else if !opts.force {
				return fmt.Errorf("o %s ja esta aberto. Ele e instancia unica, entao abrir de novo so acorda a janela antiga, que continua no IP daqui. Feche-o ou rode com -force", channel.Friendly)
			}
		}
	}

	endpoint, err := resolveProxy(opts, excluded)
	if err != nil {
		return err
	}

	country, err := ExitCountry(endpoint, probeTimeout)
	if err != nil {
		return fmt.Errorf("nao consegui confirmar o pais de saida de %s: %w", endpoint, err)
	}
	logf("proxy escolhido: %s, saida em %s", endpoint, country)

	if err := StoreCachedProxy(endpoint); err != nil {
		logf("aviso: nao consegui guardar a proxy para a proxima vez: %v", err)
	}

	if opts.checkRun {
		logf("modo -check, o Discord nao foi aberto")
		return nil
	}

	// Só encerra a instância antiga agora, com um proxy vivo na mão. Matar antes deixaria a
	// pessoa sem Discord nenhum se a busca falhasse.
	if err := terminateIfRunning(channel); err != nil {
		return err
	}

	args := append(ProxyArgs(endpoint, opts.bypass, opts.fallback), opts.extra...)
	pid, err := Launch(binary, args)
	if err != nil {
		return fmt.Errorf("nao consegui abrir o %s: %w", channel.Friendly, err)
	}

	logf("%s aberto (pid %d) com %s", channel.Friendly, pid, args[0])
	logf("dominios fora do proxy: %s", opts.bypass)
	return nil
}

func resolveTarget(opts options) (Channel, string, error) {
	if opts.channel != "auto" {
		channel, err := LookupChannel(opts.channel)
		if err != nil {
			return Channel{}, "", err
		}
		if opts.exePath != "" {
			return channel, opts.exePath, nil
		}
		binary, err := LocateDiscord(channel)
		return channel, binary, err
	}

	if opts.exePath != "" {
		return ChannelForBinary(opts.exePath), opts.exePath, nil
	}

	channel, binary, err := DetectChannel()
	if err != nil {
		return Channel{}, "", err
	}
	logf("canal detectado: %s", channel.Friendly)
	return channel, binary, nil
}

func terminateIfRunning(channel Channel) error {
	running, err := IsRunning(channel)
	if err != nil || !running {
		return nil
	}

	logf("%s ja estava aberto, encerrando", channel.Friendly)
	if err := Terminate(channel); err != nil {
		return err
	}

	// O Windows demora um pouco para soltar o lock de instância única. Subir em cima disso
	// faz o processo novo se achar duplicado e sair sozinho.
	time.Sleep(2 * time.Second)
	return nil
}

// As fontes vêm da mais previsível para a mais lenta.
func resolveProxy(opts options, excluded map[string]bool) (Endpoint, error) {
	deadline := time.Now().Add(opts.deadline)

	if opts.manual != "" {
		endpoint, ok := ParseProxy(opts.manual)
		if !ok {
			return Endpoint{}, fmt.Errorf("proxy invalido: %q. Use algo como socks5://1.2.3.4:1080", opts.manual)
		}

		latency, err := Probe(endpoint, probeTimeout)
		if err != nil {
			// Quem escolheu um proxy quer aquele, então não caímos para a busca automática:
			// ser mandado para um endereço desconhecido em silêncio é pior que o erro.
			return Endpoint{}, fmt.Errorf("seu proxy %s nao respondeu: %w", endpoint, err)
		}

		logf("seu proxy respondeu em %dms", latency.Milliseconds())
		return endpoint, nil
	}

	if !opts.noCache {
		if cached, ok := ReadCachedProxy(); ok {
			if latency, err := Probe(cached, fastProbeTimeout); err == nil {
				logf("proxy guardada revalidada em %dms: %s", latency.Milliseconds(), cached)
				return cached, nil
			}
			logf("a proxy guardada nao respondeu mais, procurando outra")
		}
	}

	if !opts.noTor {
		if tor, ok := DetectTor(excluded, logf); ok {
			return tor, nil
		}
	}

	logf("procurando uma proxy publica, isso pode demorar")
	return PickFreeProxy(excluded, deadline, logf)
}
