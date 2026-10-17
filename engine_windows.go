//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/netip"
	"sync"
	"time"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/windows/conf"
	"golang.zx2c4.com/wireguard/windows/driver"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

const (
	adapterName = "DiscordGoLiveBypass"

	monitorInterval = 2 * time.Second
	retryInterval   = 15 * time.Second
)

type EngineStatus struct {
	State     string
	Error     string
	Endpoint  string
	Routes    int
	Streams   int           // transmissões abertas com o servidor no túnel
	Handshake time.Duration // negativo enquanto nunca houve handshake
	Rx, Tx    uint64
}

// Engine mantém um túnel por vez. Start troca a configuração em uso e Stop desliga; o
// trabalho de verdade roda numa goroutine que tenta de novo sozinha quando algo falha, como
// a rede ainda não estar pronta logo depois de o Windows ligar.
type Engine struct {
	mu     sync.Mutex
	status EngineStatus
	cancel context.CancelFunc
	done   chan struct{}
	mode   string
	wake   chan struct{}
}

func NewEngine() *Engine {
	return &Engine{status: EngineStatus{State: stateOff, Handshake: -1}, mode: modeStream, wake: make(chan struct{}, 1)}
}

func (e *Engine) currentMode() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.mode
}

// SetMode troca o modo com o túnel de pé, mexendo só nas rotas que mudam. Recriar o
// adaptador faria o gateway do Discord reconectar pelo IP brasileiro no instante em que o
// túnel some, e ele só voltaria ao túnel na próxima vez que o Discord abrisse.
func (e *Engine) SetMode(mode string) {
	e.mu.Lock()
	e.mode = mode
	e.mu.Unlock()
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

func (e *Engine) Status() EngineStatus {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.status
}

func (e *Engine) update(change func(*EngineStatus)) {
	e.mu.Lock()
	before := e.status
	change(&e.status)
	after := e.status
	e.mu.Unlock()

	if before.State != after.State || before.Error != after.Error {
		if after.Error != "" {
			log.Printf("estado: %s (%s)", after.State, after.Error)
		} else {
			log.Printf("estado: %s", after.State)
		}
	}
}

// Start liga o túnel. logs são os renderer_js.log do Discord, de onde o modo transmissão
// tira quando cada Go Live abre e fecha.
func (e *Engine) Start(text, mode string, logs []string) {
	e.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	watch := newMediaWatch(discordLogSources(logs))
	e.mu.Lock()
	e.cancel, e.done, e.mode = cancel, done, mode
	e.mu.Unlock()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		watch.run(ctx)
	}()
	go func() {
		defer wg.Done()
		e.run(ctx, text, watch)
	}()
	go func() {
		wg.Wait()
		close(done)
	}()
}

func (e *Engine) Stop() {
	e.mu.Lock()
	cancel, done := e.cancel, e.done
	e.cancel, e.done = nil, nil
	e.mu.Unlock()

	if cancel != nil {
		cancel()
		<-done
	}
	e.update(func(s *EngineStatus) { *s = EngineStatus{State: stateOff, Handshake: -1} })
}

func (e *Engine) run(ctx context.Context, text string, watch *mediaWatch) {
	for {
		err := e.session(ctx, text, watch)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			e.update(func(s *EngineStatus) { s.State, s.Error = stateFailed, err.Error() })
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(retryInterval):
		}
	}
}

// Uma sessão do túnel, do adaptador subindo até o desligamento. As rotas só entram depois
// do primeiro handshake: com o servidor fora do ar, o Discord continua saindo direto, sem
// Go Live mas funcionando, em vez de ficar sem conexão nenhuma.
func (e *Engine) session(ctx context.Context, text string, watch *mediaWatch) error {
	e.update(func(s *EngineStatus) { *s = EngineStatus{State: stateConnecting, Handshake: -1} })

	cfg, err := parseConf(text, "da VPN")
	if err != nil {
		return err
	}
	if err := cfg.ResolveEndpoints(); err != nil {
		return fmt.Errorf("não consegui resolver o endereço do servidor da VPN: %v", err)
	}

	// A conferência usa as rotas do modo completo, que cobrem tudo o que algum dia pode
	// entrar no túnel, para a troca de modo não esbarrar no servidor da VPN depois.
	resolved := ResolveDiscordHosts(5 * time.Second)
	if err := checkEndpoints(cfg, routesFor(modeFull, resolved)); err != nil {
		return err
	}
	staticFor := func(mode string) []netip.Prefix {
		if mode == modeStream && !watch.available() {
			log.Printf("sem o log do Discord para ler, o modo transmissão fica igual ao completo")
			mode = modeFull
		}
		return routesFor(mode, resolved)
	}
	// O servidor de uma transmissão vem do log, e um endereço que por acaso fosse o do
	// próprio servidor da VPN derrubaria o túnel.
	endpoints := endpointAddrs(cfg)
	dynamic := func() ([]netip.Prefix, int) {
		routes, streams := watch.state()
		return exclude(routes, endpoints), streams
	}
	if cfg.Interface.MTU == 0 {
		cfg.Interface.MTU = defaultMTU
	}
	for i := range cfg.Peers {
		if cfg.Peers[i].PersistentKeepalive == 0 {
			cfg.Peers[i].PersistentKeepalive = defaultKeepalive
		}
	}

	adapter, err := createAdapter()
	if err != nil {
		return fmt.Errorf("não consegui criar o adaptador de rede do túnel: %v", err)
	}
	defer adapter.Close()

	if running, err := driver.RunningVersion(); err == nil {
		log.Printf("WireGuardNT %d.%d", (running>>16)&0xffff, running&0xffff)
	}
	if err := adapter.SetLogging(driver.AdapterLogOn); err != nil {
		log.Printf("sem log do driver: %v", err)
	}

	// O aviso entra antes de o adaptador subir: registrado depois, uma interface IPv6 que
	// aparecesse no meio do caminho ficaria sem configuração.
	mode := e.currentMode()
	network := &tunnelNetwork{luid: adapter.LUID(), cfg: cfg, static: staticFor(mode), applied: map[netip.Prefix]bool{}}
	var streams int
	network.dynamic, streams = dynamic()
	callback, err := winipcfg.RegisterInterfaceChangeCallback(func(kind winipcfg.MibNotificationType, iface *winipcfg.MibIPInterfaceRow) {
		if kind != winipcfg.MibAddInstance || iface.InterfaceLUID != network.luid {
			return
		}
		family := iface.Family
		go func() {
			if err := network.configureFamily(family); err != nil {
				log.Printf("reconfigurando a família %d: %v", family, err)
			}
		}()
	})
	if err != nil {
		log.Printf("sem aviso de mudança de interface: %v", err)
	} else {
		defer callback.Unregister()
	}

	if err := adapter.SetConfiguration(cfg.ToDriverConfiguration()); err != nil {
		return fmt.Errorf("o driver recusou a configuração da VPN: %v", err)
	}
	if err := adapter.SetAdapterState(driver.AdapterStateUp); err != nil {
		return fmt.Errorf("não consegui ligar o adaptador do túnel: %v", err)
	}
	if err := network.configure(); err != nil {
		return err
	}

	endpoint := cfg.Peers[0].Endpoint.String()
	e.update(func(s *EngineStatus) { s.Endpoint, s.Routes, s.Streams = endpoint, network.count(), streams })
	log.Printf("adaptador ligado, servidor %s, modo %s, %d rotas esperando o primeiro handshake", endpoint, mode, network.count())

	started := time.Now()
	ticker := time.NewTicker(monitorInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-e.wake:
			if next := e.currentMode(); next != mode {
				mode = next
				if err := network.setStatic(staticFor(mode)); err != nil {
					log.Printf("trocando para o modo %s: %v", mode, err)
				}
				log.Printf("modo %s, %d rotas", mode, network.count())
				e.update(func(s *EngineStatus) { s.Routes = network.count() })
			}
			continue
		case <-watch.changed:
			routes, streams := dynamic()
			if err := network.setDynamic(routes); err != nil {
				log.Printf("rotas da transmissão: %v", err)
			}
			e.update(func(s *EngineStatus) { s.Routes, s.Streams = network.count(), streams })
			continue
		case <-ticker.C:
		}

		if state, err := adapter.AdapterState(); err == nil && state == driver.AdapterStateDown {
			log.Printf("o adaptador caiu, religando")
			if err := adapter.SetConfiguration(cfg.ToDriverConfiguration()); err != nil {
				log.Printf("religar, configuração: %v", err)
			}
			if err := adapter.SetAdapterState(driver.AdapterStateUp); err != nil {
				log.Printf("religar, estado: %v", err)
			}
		}

		age, rx, tx := peerStats(adapter)
		switch routeDecision(network.isRouted(), age) {
		case addRoutes:
			if err := network.setRouted(true); err != nil {
				return fmt.Errorf("não consegui pôr as rotas do Discord no túnel: %v", err)
			}
			log.Printf("servidor respondeu, rotas do Discord no túnel")
		case dropRoutes:
			if err := network.setRouted(false); err != nil {
				log.Printf("tirando as rotas: %v", err)
			}
			log.Printf("servidor sem resposta há %s, Discord saindo direto até ele voltar", age.Round(time.Second))
		}

		state := sessionState(network.isRouted(), age, time.Since(started))
		e.update(func(s *EngineStatus) {
			s.State, s.Handshake, s.Rx, s.Tx = state, age, rx, tx
			s.Error = ""
			if state == stateNoServer {
				s.Error = "o servidor da VPN não está respondendo. O Discord continua funcionando, sem Go Live, e o túnel volta sozinho quando o servidor responder"
			}
		})
	}
}

// O pacote do driver entra em pânico quando não acha o wireguard.dll, em vez de devolver
// erro. Só acontece num build feito à mão sem o recurso embutido, mas aí a mensagem
// precisa chegar à janela, e não o serviço cair calado.
func createAdapter() (adapter *driver.Adapter, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("o driver do WireGuard não carregou (%v). Compile pelo Compilar.ps1, que embute o driver no executável", r)
		}
	}()
	return driver.CreateAdapter(adapterName, adapterName, nil)
}

func endpointAddrs(cfg *conf.Config) []netip.Addr {
	var addrs []netip.Addr
	for i := range cfg.Peers {
		if addr, err := netip.ParseAddr(cfg.Peers[i].Endpoint.Host); err == nil {
			addrs = append(addrs, addr.Unmap())
		}
	}
	return addrs
}

func peerStats(adapter *driver.Adapter) (age time.Duration, rx, tx uint64) {
	iface, err := adapter.Configuration()
	if err != nil || iface.PeerCount == 0 {
		return -1, 0, 0
	}
	peer := iface.FirstPeer()
	if peer.LastHandshake == 0 {
		return -1, peer.RxBytes, peer.TxBytes
	}
	// FILETIME: intervalos de 100 ns desde 1601.
	const unixEpoch = 116444736000000000
	at := time.Unix(0, int64(peer.LastHandshake-unixEpoch)*100)
	return time.Since(at), peer.RxBytes, peer.TxBytes
}

// Endereços e rotas do adaptador. O Windows pode descartar a configuração quando a
// interface some e volta, por suspensão ou troca de rede, então ela fica guardada aqui
// para ser reaplicada.
type tunnelNetwork struct {
	mu      sync.Mutex
	luid    winipcfg.LUID
	cfg     *conf.Config
	static  []netip.Prefix // as do modo
	dynamic []netip.Prefix // as das transmissões abertas
	routed  bool
	applied map[netip.Prefix]bool
	warned6 bool
}

func (n *tunnelNetwork) isRouted() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.routed
}

func familyOf(prefix netip.Prefix) winipcfg.AddressFamily {
	if prefix.Addr().Is4() {
		return windows.AF_INET
	}
	return windows.AF_INET6
}

func nextHopFor(family winipcfg.AddressFamily) netip.Addr {
	if family == windows.AF_INET {
		return netip.IPv4Unspecified()
	}
	return netip.IPv6Unspecified()
}

func (n *tunnelNetwork) wanted() map[netip.Prefix]bool {
	want := map[netip.Prefix]bool{}
	for _, prefix := range n.static {
		want[prefix.Masked()] = true
	}
	for _, prefix := range n.dynamic {
		want[prefix.Masked()] = true
	}
	return want
}

func (n *tunnelNetwork) count() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.wanted())
}

// syncLocked põe e tira rota por rota até a tabela bater com o pedido, e não troca a tabela
// inteira da interface: assim as rotas que o próprio Windows cria para os endereços do
// adaptador ficam onde estão. As novas entram antes de as velhas saírem, e a transmissão
// não passa nem um instante pelo caminho normal quando a faixa larga vira o /32 dela.
func (n *tunnelNetwork) syncLocked() error {
	want := map[netip.Prefix]bool{}
	if n.routed {
		want = n.wanted()
	}
	add, del := diffRoutes(n.applied, want)

	var firstErr error
	fail := func(prefix netip.Prefix, err error) {
		// IPv6 é melhor esforço: numa máquina com o IPv6 desligado não há onde pôr essas
		// rotas, e aí também não há por onde o Discord escapar em IPv6.
		if familyOf(prefix) == windows.AF_INET6 {
			if !n.warned6 {
				n.warned6 = true
				log.Printf("rotas IPv6: %s: %v", prefix, err)
			}
			return
		}
		if firstErr == nil {
			firstErr = fmt.Errorf("%s: %w", prefix, err)
		}
	}
	for _, prefix := range add {
		err := n.luid.AddRoute(prefix, nextHopFor(familyOf(prefix)), 0)
		if err != nil && !errors.Is(err, windows.ERROR_OBJECT_ALREADY_EXISTS) {
			fail(prefix, err)
			continue
		}
		n.applied[prefix] = true
	}
	for _, prefix := range del {
		err := n.luid.DeleteRoute(prefix, nextHopFor(familyOf(prefix)))
		if err != nil && !errors.Is(err, windows.ERROR_NOT_FOUND) {
			fail(prefix, err)
			continue
		}
		delete(n.applied, prefix)
	}
	return firstErr
}

func (n *tunnelNetwork) setRouted(on bool) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.routed = on
	return n.syncLocked()
}

func (n *tunnelNetwork) setStatic(routes []netip.Prefix) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.static = routes
	return n.syncLocked()
}

func (n *tunnelNetwork) setDynamic(routes []netip.Prefix) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.dynamic = routes
	return n.syncLocked()
}

func (n *tunnelNetwork) configureFamily(family winipcfg.AddressFamily) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	var addresses []netip.Prefix
	for _, address := range n.cfg.Interface.Addresses {
		if (address.Addr().Is4() && family == windows.AF_INET) || (address.Addr().Is6() && family == windows.AF_INET6) {
			addresses = append(addresses, address)
		}
	}
	if len(addresses) > 0 {
		err := n.luid.SetIPAddressesForFamily(family, addresses)
		if errors.Is(err, windows.ERROR_OBJECT_ALREADY_EXISTS) {
			cleanupStaleAddresses(family, addresses)
			err = n.luid.SetIPAddressesForFamily(family, addresses)
		}
		if err != nil {
			return fmt.Errorf("endereços: %w", err)
		}
	}

	iface, err := n.luid.IPInterface(family)
	if err != nil {
		return fmt.Errorf("interface: %w", err)
	}
	iface.RouterDiscoveryBehavior = winipcfg.RouterDiscoveryDisabled
	iface.DadTransmits = 0
	iface.ManagedAddressConfigurationSupported = false
	iface.OtherStatefulConfigurationSupported = false
	iface.NLMTU = uint32(n.cfg.Interface.MTU)
	if err := iface.Set(); err != nil {
		return fmt.Errorf("mtu: %w", err)
	}

	// A interface que volta do zero volta sem as rotas dessa família.
	for prefix := range n.applied {
		if familyOf(prefix) == family {
			delete(n.applied, prefix)
		}
	}
	if n.routed {
		return n.syncLocked()
	}
	return nil
}

// A interface IPv4 do adaptador aparece um instante depois de ele subir, e até lá o
// Windows responde "não encontrado". O mesmo prazo de um minuto do cliente oficial.
func (n *tunnelNetwork) configure() error {
	deadline := time.Now().Add(time.Minute)
	for {
		err := n.configureFamily(windows.AF_INET)
		if err == nil {
			break
		}
		if !errors.Is(err, windows.ERROR_NOT_FOUND) || time.Now().After(deadline) {
			return fmt.Errorf("não consegui configurar o adaptador do túnel: %w", err)
		}
		time.Sleep(250 * time.Millisecond)
	}
	if err := n.configureFamily(windows.AF_INET6); err != nil {
		log.Printf("IPv6 do adaptador ainda não configurado: %v", err)
	}
	return nil
}

// Endereço igual preso num adaptador desligado, como o de um app de VPN instalado e
// desconectado, faz o Windows recusar o nosso. O cliente oficial faz a mesma limpeza.
func cleanupStaleAddresses(family winipcfg.AddressFamily, addresses []netip.Prefix) {
	wanted := map[netip.Addr]bool{}
	for _, address := range addresses {
		wanted[address.Addr()] = true
	}

	interfaces, err := winipcfg.GetAdaptersAddresses(family, winipcfg.GAAFlagDefault)
	if err != nil {
		return
	}
	for _, iface := range interfaces {
		if iface.OperStatus == winipcfg.IfOperStatusUp {
			continue
		}
		for address := iface.FirstUnicastAddress; address != nil; address = address.Next {
			ip, _ := netip.AddrFromSlice(address.Address.IP())
			if wanted[ip.Unmap()] {
				prefix := netip.PrefixFrom(ip.Unmap(), int(address.OnLinkPrefixLength))
				log.Printf("removendo %s preso no adaptador desligado %s", prefix, iface.FriendlyName())
				iface.LUID.DeleteIPAddress(prefix)
			}
		}
	}
}
