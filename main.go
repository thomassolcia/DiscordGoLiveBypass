package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Tempo que o processo precisa sobreviver para a abertura contar como bem-sucedida. Quem
// morre por instalação quebrada morre bem antes disso.
const survivalDelay = 3 * time.Second

// A conta gratuita do Proton gera o arquivo do WireGuard nessa página. Qualquer outro
// provedor que entregue um .conf serve do mesmo jeito.
const protonDownloads = "https://account.protonvpn.com/downloads"

type options struct {
	conf     string
	exclude  string
	channel  string
	exePath  string
	force    bool
	noUpdate bool
	checkRun bool
	extra    []string

	// Só o processo elevado que mantém o túnel usa estes.
	tunnel bool
	state  string
	watch  string
}

// Opções do proxy da 1.x. Continuam aceitas para um atalho antigo não quebrar com "flag
// provided but not defined", mas só avisam que não fazem mais nada. O valor diz se a
// opção era liga/desliga, que o pacote flag precisa saber para não engolir o argumento
// seguinte.
var retiredFlags = map[string]bool{
	"proxy":    false,
	"bypass":   false,
	"route":    false,
	"timeout":  false,
	"no-cache": true,
	"no-tor":   true,
	"fallback": true,
}

var internalFlags = map[string]bool{"tunnel": true, "state": true, "watch": true}

type retiredFlag struct {
	name   string
	isBool bool
	used   *[]string
}

func (f retiredFlag) String() string   { return "" }
func (f retiredFlag) IsBoolFlag() bool { return f.isBool }
func (f retiredFlag) Set(string) error {
	*f.used = append(*f.used, f.name)
	return nil
}

func main() {
	opts := options{}
	var retired []string

	flag.StringVar(&opts.conf, "conf", "", "arquivo WireGuard (.conf) da sua VPN; fica guardado para as proximas vezes")
	flag.StringVar(&opts.exclude, "exclude", "BR", "codigos de pais recusados na saida, separados por virgula")
	flag.StringVar(&opts.channel, "channel", "auto", "canal do Discord: auto, stable, ptb ou canary")
	flag.StringVar(&opts.exePath, "exe", "", "caminho explicito do executavel, ignora a deteccao automatica")
	flag.BoolVar(&opts.force, "force", false, "encerra um Discord ja aberto antes de subir o novo")
	flag.BoolVar(&opts.noUpdate, "no-update", false, "nao consulta o GitHub atras de versao nova")
	flag.BoolVar(&opts.checkRun, "check", false, "so liga o tunel e confere a saida, nao abre o Discord")
	flag.BoolVar(&opts.tunnel, "tunnel", false, "uso interno")
	flag.StringVar(&opts.state, "state", "", "uso interno")
	flag.StringVar(&opts.watch, "watch", "", "uso interno")
	for name, isBool := range retiredFlags {
		flag.Var(retiredFlag{name, isBool, &retired}, name, "removida na 2.0")
	}

	flag.Usage = usage
	flag.Parse()
	opts.extra = flag.Args()

	if opts.tunnel {
		if err := runTunnelHelper(opts.conf, opts.state, opts.watch); err != nil {
			os.Exit(1)
		}
		return
	}

	ui := NewUI()
	newer := CheckUpdate(!opts.noUpdate)

	// O aviso sai por ultimo nos dois desfechos. Numa falha ele importa ainda mais: rodar
	// uma versao velha e um motivo plausivel para o que acabou de dar errado.
	sort.Strings(retired)
	err := run(opts, ui, retired)
	if err != nil {
		ui.Fatal(err)
	}
	announceUpdate(ui, newer)
	ui.Close()

	if err != nil {
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `DiscordGoLiveBypass: abre o Discord com so ele passando por uma VPN WireGuard fora do Brasil.

uso: %s [flags] [-- argumentos extras para o Discord]

`, os.Args[0])

	flag.VisitAll(func(f *flag.Flag) {
		if internalFlags[f.Name] {
			return
		}
		if _, old := retiredFlags[f.Name]; old {
			return
		}
		name, text := flag.UnquoteUsage(f)
		line := "  -" + f.Name
		if name != "" {
			line += " " + name
		}
		fmt.Fprintf(os.Stderr, "%s\n    \t%s", line, text)
		if f.DefValue != "" && f.DefValue != "false" {
			fmt.Fprintf(os.Stderr, " (padrao %q)", f.DefValue)
		}
		fmt.Fprintln(os.Stderr)
	})
}

func run(opts options, ui *UI, retired []string) error {
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
	for _, name := range retired {
		ui.Warn("a opcao -%s era do proxy da 1.x e nao faz mais nada na 2.0", name)
	}

	// Resolver o executável antes do túnel evita pedir administrador e subir um adaptador
	// de rede para só então descobrir que o Discord nem está instalado.
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

		running, err := IsRunning(channel)
		switch {
		case err != nil && opts.checkRun:
			ui.Warn("nao consegui checar se o %s ja esta aberto: %v", channel.Friendly, err)
		case err != nil:
			return fmt.Errorf("nao consegui checar se o %s ja esta aberto: %w. Sem essa resposta nao da para abrir: se ele estiver de pe, as conexoes antigas dele continuam pela rota de antes e a tela mostraria sucesso do mesmo jeito", channel.Friendly, err)
		case running && opts.checkRun:
			ui.Warn("o %s esta aberto agora, uma abertura de verdade pediria -force", channel.Friendly)
		case running && !opts.force:
			return fmt.Errorf("o %s ja esta aberto, e as conexoes que ele ja tem continuariam pela rota de antes. Feche-o ou rode com -force", channel.Friendly)
		case running:
			ui.Warn("o %s ja esta aberto, sera reiniciado no fim", channel.Friendly)
		}
	}

	ui.Step("Procurando o seu arquivo de VPN")
	confPath, err := resolveConf(opts, ui)
	if err != nil {
		return err
	}
	summary, err := ValidateConf(confPath)
	if err != nil {
		return err
	}
	ui.Ok("configuracao WireGuard valida, servidor %s", summary.Endpoint)

	ui.Step("Ligando o tunel so para o Discord")
	watch := ""
	if binary != "" {
		watch = filepath.Base(binary)
	}
	tun, err := ensureTunnel(confPath, watch, excluded, ui)
	if err != nil {
		return err
	}
	ui.Ok("o Discord ve a saida em %s, %d ms", CountryLabel(tun.exit.Country), tun.exit.Latency.Milliseconds())

	rows := [][2]string{
		{"saida", CountryLabel(tun.exit.Country)},
		{"ip visto", tun.exit.IP},
		{"servidor", summary.Endpoint},
		{"resposta", fmt.Sprintf("%d ms", tun.exit.Latency.Milliseconds())},
	}

	if opts.checkRun {
		if !tun.reused {
			tun.stop()
			rows = append(rows, [2]string{"tunel", "desligado de novo, modo -check"})
		}
		ui.Summary("tunel conferido", append(rows, [2]string{"discord", "nao foi aberto, modo -check"}))
		return nil
	}

	// Daqui para baixo, uma falha deixaria o túnel ligado esperando um Discord que não vem.
	opened := false
	defer func() {
		if !opened && !tun.reused {
			tun.stop()
		}
	}()

	ui.Step("Abrindo o %s", channel.Friendly)
	if err := terminateIfRunning(channel, ui); err != nil {
		return err
	}

	ui.Busy("abrindo o %s", channel.Friendly)
	pid, err := launchDiscord(binary, opts.extra, ui)
	if err != nil {
		return fmt.Errorf("nao consegui abrir o %s: %w", channel.Friendly, err)
	}
	// O Start só prova que o processo nasceu. Sem conferir se ele sobreviveu, uma instalação
	// quebrada faz a tela mostrar a caixa verde com nada aberto.
	ui.Busy("conferindo se o %s continua de pe", channel.Friendly)
	time.Sleep(survivalDelay)
	if alive, err := IsRunning(channel); err == nil && !alive {
		return fmt.Errorf("o %s abriu e morreu logo em seguida, sem chegar a mostrar janela. Costuma ser instalacao incompleta: abra o %s normalmente uma vez, deixe ele terminar de se atualizar e rode de novo", channel.Friendly, channel.Friendly)
	}
	opened = true

	opening := channel.Friendly
	if pid > 0 {
		opening = fmt.Sprintf("%s, pid %d", channel.Friendly, pid)
	}
	ui.Ok("%s aberto", opening)

	rows = append(rows,
		[2]string{"discord", opening},
		[2]string{"tunel", "fica ligado enquanto o Discord estiver aberto"})
	ui.Summary("tudo pronto", rows)
	return nil
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
	if err != nil {
		return fmt.Errorf("nao consegui checar se o %s esta aberto para encerra-lo: %w", channel.Friendly, err)
	}
	if !running {
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

func launchDiscord(binary string, extra []string, ui *UI) (int, error) {
	if !isElevated() {
		return Launch(binary, extra)
	}
	if len(extra) > 0 {
		ui.Warn("rodando como administrador, os argumentos extras nao chegam ao Discord")
	}
	return 0, launchUnelevated(binary)
}

var errNoConf = errors.New("nao achei nenhum arquivo de VPN (.conf do WireGuard). Rode pelo Abrir-Discord.bat para ver o passo a passo, ou aponte o arquivo com -conf")

func resolveConf(opts options, ui *UI) (string, error) {
	if opts.conf != "" {
		if _, err := os.Stat(opts.conf); err != nil {
			return "", fmt.Errorf("nao achei o arquivo %s", opts.conf)
		}
		ui.Ok("usando o arquivo que voce passou")
		return keepConf(opts.conf, ui), nil
	}

	if stored, err := storedConfPath(); err == nil {
		if _, err := os.Stat(stored); err == nil {
			ui.Ok("usando o arquivo guardado da primeira vez")
			ui.Detail("%s", stored)
			return stored, nil
		}
	}

	return firstRunConf(ui)
}

// Falhar em guardar a cópia não impede esta abertura, só a próxima vai perguntar de novo.
func keepConf(path string, ui *UI) string {
	stored, err := storeConf(path)
	if err != nil {
		ui.Warn("nao consegui guardar uma copia para as proximas vezes: %v", err)
		return path
	}
	ui.Detail("copia guardada em %s, as proximas aberturas nao perguntam mais", stored)
	return stored
}

func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(exe)
}

// Primeira abertura: procura o arquivo onde o navegador costuma salvar e, se não houver,
// explica como conseguir um. Isso acontece uma vez só, depois a cópia guardada basta.
func firstRunConf(ui *UI) (string, error) {
	dirs := []string{downloadsDir(), exeDir()}

	for _, candidate := range findConfCandidates(dirs) {
		ui.Info("achei um arquivo do WireGuard: %s", filepath.Base(candidate.Path))
		ui.Detail("em %s, de %s", filepath.Dir(candidate.Path), candidate.Modified.Format("02/01/2006 15:04"))
		answer, ok := ui.Ask("Usar este arquivo? [S/n] ")
		if !ok {
			return "", errNoConf
		}
		if yes(answer) {
			return keepConf(candidate.Path, ui), nil
		}
	}

	ui.Info("falta o arquivo da VPN por onde o Discord vai sair. E so desta vez:")
	ui.Detail("1. crie uma conta gratuita no Proton VPN, em protonvpn.com")
	ui.Detail("2. entre em account.protonvpn.com e abra Downloads")
	ui.Detail("3. em WireGuard configuration, escolha Windows e um servidor gratuito")
	ui.Detail("4. clique em Create e baixe o arquivo .conf")
	ui.Detail("qualquer outra VPN que entregue um arquivo WireGuard tambem serve")

	answer, ok := ui.Ask("Abrir a pagina do Proton no navegador? [S/n] ")
	if !ok {
		return "", errNoConf
	}
	if yes(answer) {
		if err := openBrowser(protonDownloads); err != nil {
			ui.Warn("nao consegui abrir o navegador, entre em %s", protonDownloads)
		}
	}

	for {
		answer, ok := ui.Ask("Com o arquivo baixado, aperte Enter (ou arraste o .conf para esta janela): ")
		if !ok {
			return "", errNoConf
		}

		if path := strings.Trim(strings.TrimSpace(answer), `"`); path != "" {
			raw, err := os.ReadFile(path)
			if err == nil && looksLikeWireGuard(string(raw)) {
				return keepConf(path, ui), nil
			}
			ui.Warn("esse arquivo nao parece uma configuracao WireGuard")
			continue
		}

		if found := findConfCandidates(dirs); len(found) > 0 {
			ui.Ok("achei %s", filepath.Base(found[0].Path))
			return keepConf(found[0].Path, ui), nil
		}
		ui.Warn("ainda nao achei nenhum .conf do WireGuard em %s", dirs[0])
	}
}

type tunnelResult struct {
	exit   exitInfo
	reused bool
	pid    int
	dir    string
}

func (t tunnelResult) stop() {
	if requestTunnelStop(t.dir) == nil {
		waitTunnelGone(t.pid, 10*time.Second)
	}
}

func ensureTunnel(confPath, watch string, excluded map[string]bool, ui *UI) (tunnelResult, error) {
	dir, err := dataDir()
	if err != nil {
		return tunnelResult{}, fmt.Errorf("nao achei a pasta de dados do usuario: %w", err)
	}
	hash, err := confHash(confPath)
	if err != nil {
		return tunnelResult{}, fmt.Errorf("nao consegui ler o arquivo de VPN: %w", err)
	}

	// Reabrir o Discord com o túnel da última vez ainda de pé não precisa de outro pedido
	// de administrador, desde que a configuração seja a mesma e ele continue respondendo.
	if old, ok := readTunnelState(dir); ok && old.Phase == phaseReady && helperAlive(old.PID) {
		if old.ConfHash == hash {
			touchHold(dir)
			ui.Busy("o tunel da ultima abertura continua ligado, conferindo a saida")
			exit, err := verifyExit(excluded)
			if err == nil {
				ui.Ok("tunel da ultima abertura reaproveitado")
				return tunnelResult{exit, true, old.PID, dir}, nil
			}
			ui.Warn("o tunel que estava ligado nao serve mais (%v), vou subir outro", err)
		}
		ui.Busy("desligando o tunel anterior")
		tunnelResult{pid: old.PID, dir: dir}.stop()
	}

	ui.Info("o Windows vai pedir permissao de administrador, e ela que deixa criar o adaptador de rede do tunel")
	ui.Busy("esperando a permissao de administrador")
	args := []string{"-tunnel", "-conf", confPath, "-state", dir}
	if watch != "" {
		args = append(args, "-watch", watch)
	}
	pid, err := startTunnelHelper(args)
	if errors.Is(err, errElevationDenied) {
		return tunnelResult{}, errors.New("a permissao de administrador foi negada. Sem ela o Windows nao deixa criar o adaptador de rede do tunel: rode de novo e responda Sim no aviso")
	}
	if err != nil {
		return tunnelResult{}, fmt.Errorf("nao consegui iniciar o processo do tunel: %w", err)
	}
	tun := tunnelResult{pid: pid, dir: dir}

	ui.Busy("subindo o tunel")
	state, err := waitTunnelReady(dir, pid, time.Minute)
	if err != nil {
		tun.stop()
		return tun, fmt.Errorf("o tunel nao subiu: %v. O registro completo fica em %s", err, logPath(dir))
	}
	ui.Ok("tunel ligado, %d faixas de IP do Discord passam por ele", state.Routes)

	ui.Busy("conferindo por onde o Discord vai sair")
	tun.exit, err = verifyExit(excluded)
	if err != nil {
		tun.stop()
		return tun, err
	}
	return tun, nil
}

// O primeiro pedido pelo túnel é o que dispara o handshake com o servidor, então uma falha
// logo de cara ainda não quer dizer nada.
func verifyExit(excluded map[string]bool) (exitInfo, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		exit, err := CheckExit(10 * time.Second)
		if err == nil {
			if excluded[exit.Country] {
				return exit, fmt.Errorf("o Discord ainda ve a saida em %s. Ou o servidor do arquivo de VPN fica num pais recusado, e ai vale gerar outro com um servidor de fora, ou as rotas do tunel nao pegaram: o registro fica na pasta de dados, em tunnel.log", CountryLabel(exit.Country))
			}
			return exit, nil
		}
		lastErr = err
		time.Sleep(time.Second)
	}
	return exitInfo{}, fmt.Errorf("o tunel subiu, mas o Discord nao respondeu por ele: %s. Costuma ser servidor da VPN fora do ar ou arquivo .conf antigo: gere outro no site da VPN e rode com -conf", DescribeNetwork(lastErr))
}
