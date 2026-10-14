//go:build windows

package main

import (
	"errors"
	"fmt"
	"log"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/windows/conf"
	"golang.zx2c4.com/wireguard/windows/driver"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

const (
	defaultMTU = 1420
	// Sem keepalive, o mapeamento do roteador de casa expira num silêncio mais longo e o
	// servidor perde o caminho de volta. Arquivo que já traz um valor fica com o dele.
	defaultKeepalive = 25
	maxLogBytes      = 1024 * 1024
)

func loadConf(path string) (*conf.Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("nao consegui ler o arquivo de VPN %s: %w", path, err)
	}
	if len(raw) > maxConfBytes {
		return nil, fmt.Errorf("o arquivo %s e grande demais para ser uma configuracao WireGuard", filepath.Base(path))
	}

	cfg, err := conf.FromWgQuickWithUnknownEncoding(string(raw), adapterName)
	if err != nil {
		return nil, fmt.Errorf("o arquivo %s nao e uma configuracao WireGuard valida (%s). Gere outro no site da sua VPN e rode com -conf", filepath.Base(path), redactKeys(err.Error(), string(raw)))
	}
	if len(cfg.Peers) == 0 {
		return nil, fmt.Errorf("o arquivo %s nao tem nenhum servidor, falta a secao [Peer]", filepath.Base(path))
	}
	for i := range cfg.Peers {
		if cfg.Peers[i].Endpoint.IsEmpty() {
			return nil, fmt.Errorf("o arquivo %s nao diz o endereco do servidor, falta o Endpoint no [Peer]", filepath.Base(path))
		}
	}

	// O Discord conversa por IPv4. Um túnel só com IPv6 subiria sem erro e não levaria nada.
	hasIPv4 := false
	for _, address := range cfg.Interface.Addresses {
		if address.Addr().Is4() {
			hasIPv4 = true
		}
	}
	if !hasIPv4 {
		return nil, fmt.Errorf("o arquivo %s nao tem um endereco IPv4 no Address do [Interface]", filepath.Base(path))
	}
	return cfg, nil
}

// O parser oficial repete na mensagem o trecho que recusou, e quando o problema é a chave
// esse trecho é a própria chave. Ela sai da mensagem antes de ir para a tela ou para o
// log. Valor curto demais não é chave de verdade e trocá-lo picotaria o resto do texto.
func redactKeys(message, raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		name, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "privatekey", "presharedkey":
		default:
			continue
		}
		if comment := strings.Index(value, "#"); comment >= 0 {
			value = value[:comment]
		}
		if value = strings.TrimSpace(value); len(value) >= 8 {
			message = strings.ReplaceAll(message, value, "<chave omitida>")
		}
	}
	return message
}

func summarize(cfg *conf.Config) confSummary {
	summary := confSummary{Endpoint: cfg.Peers[0].Endpoint.String()}
	for _, address := range cfg.Interface.Addresses {
		if address.Addr().Is4() {
			summary.Address = address.String()
			break
		}
	}
	return summary
}

// Se o servidor da VPN ficasse dentro das faixas do Discord, o próprio túnel tentaria
// passar por dentro dele mesmo e nada sairia.
func checkEndpoints(cfg *conf.Config, routes []netip.Prefix) error {
	for i := range cfg.Peers {
		addr, err := netip.ParseAddr(cfg.Peers[i].Endpoint.Host)
		if err == nil && covered(routes, addr) {
			return fmt.Errorf("o servidor da VPN (%s) fica dentro da rede do Discord, e o tunel nao consegue passar por dentro dele mesmo. Use um arquivo de outro servidor", cfg.Peers[i].Endpoint.Host)
		}
	}
	return nil
}

func ValidateConf(path string) (confSummary, error) {
	cfg, err := loadConf(path)
	if err != nil {
		return confSummary{}, err
	}
	if err := checkEndpoints(cfg, discordPrefixes); err != nil {
		return confSummary{}, err
	}
	return summarize(cfg), nil
}

// Endereços e rotas do adaptador. O Windows pode descartar a configuração quando a
// interface some e volta, por suspensão ou troca de rede, então ela fica guardada aqui
// para ser reaplicada.
type tunnelNetwork struct {
	mu     sync.Mutex
	luid   winipcfg.LUID
	cfg    *conf.Config
	routes []netip.Prefix
}

func (n *tunnelNetwork) configureFamily(family winipcfg.AddressFamily) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	want4 := family == windows.AF_INET

	var routes []*winipcfg.RouteData
	for _, prefix := range n.routes {
		if prefix.Addr().Is4() != want4 {
			continue
		}
		nextHop := netip.IPv4Unspecified()
		if !want4 {
			nextHop = netip.IPv6Unspecified()
		}
		routes = append(routes, &winipcfg.RouteData{Destination: prefix.Masked(), NextHop: nextHop})
	}

	var addresses []netip.Prefix
	for _, address := range n.cfg.Interface.Addresses {
		if address.Addr().Is4() == want4 {
			addresses = append(addresses, address)
		}
	}

	if len(routes) > 0 {
		if err := n.luid.SetRoutesForFamily(family, routes); err != nil {
			return fmt.Errorf("rotas: %w", err)
		}
	}
	if len(addresses) > 0 {
		err := n.luid.SetIPAddressesForFamily(family, addresses)
		if errors.Is(err, windows.ERROR_OBJECT_ALREADY_EXISTS) {
			cleanupStaleAddresses(family, addresses)
			err = n.luid.SetIPAddressesForFamily(family, addresses)
		}
		if err != nil {
			return fmt.Errorf("enderecos: %w", err)
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
			return fmt.Errorf("nao consegui configurar o adaptador do tunel: %w", err)
		}
		time.Sleep(250 * time.Millisecond)
	}

	// IPv6 é melhor esforço: o arquivo do Proton não traz endereço IPv6, e aí as rotas só
	// servem para o Discord desistir rápido do IPv6 e cair no IPv4, que passa pelo túnel.
	if err := n.configureFamily(windows.AF_INET6); err != nil {
		log.Printf("rotas IPv6 nao aplicadas agora: %v", err)
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

func openTunnelLog(dir string) *os.File {
	path := logPath(dir)
	if info, err := os.Stat(path); err == nil && info.Size() > maxLogBytes {
		os.Remove(path)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil
	}
	return file
}

// Um túnel de uma abertura anterior ainda de pé seguraria o mesmo nome de adaptador e as
// mesmas rotas.
func retireOldHelper(dir string) {
	old, ok := readTunnelState(dir)
	if !ok || old.PID == os.Getpid() || !helperAlive(old.PID) {
		return
	}

	log.Printf("encerrando o tunel anterior, pid %d", old.PID)
	if err := terminateProcess(old.PID); err != nil {
		log.Printf("nao consegui encerrar o pid %d: %v", old.PID, err)
		return
	}
	waitTunnelGone(old.PID, 5*time.Second)
}

// O pacote do driver entra em pânico quando não acha o wireguard.dll, em vez de devolver
// erro. Só acontece num build feito à mão sem o recurso embutido, mas aí a mensagem
// precisa chegar ao launcher, e não o processo do túnel sumir calado.
func createAdapter() (adapter *driver.Adapter, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("o driver do WireGuard nao carregou (%v). Compile pelo Compilar.ps1, que embute o driver no executavel", r)
		}
	}()
	return driver.CreateAdapter(adapterName, adapterName, nil)
}

func handshakeAge(adapter *driver.Adapter) string {
	iface, err := adapter.Configuration()
	if err != nil {
		return fmt.Sprintf("sem leitura (%v)", err)
	}
	peer := iface.FirstPeer()
	if peer == nil || iface.PeerCount == 0 {
		return "sem servidor"
	}
	if peer.LastHandshake == 0 {
		return fmt.Sprintf("nenhum ainda, rx %d tx %d", peer.RxBytes, peer.TxBytes)
	}

	// FILETIME: intervalos de 100 ns desde 1601.
	const unixEpoch = 116444736000000000
	at := time.Unix(0, int64(peer.LastHandshake-unixEpoch)*100)
	return fmt.Sprintf("ha %s, rx %d tx %d", time.Since(at).Round(time.Second), peer.RxBytes, peer.TxBytes)
}

func runTunnelHelper(confPath, dir, watch string) error {
	if dir == "" {
		var err error
		if dir, err = dataDir(); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	if logFile := openTunnelLog(dir); logFile != nil {
		defer logFile.Close()
		log.SetOutput(logFile)
	}
	log.SetFlags(log.Ldate | log.Ltime)
	log.Printf("tunel iniciando, pid %d, versao %s, vigiando %q", os.Getpid(), version, watch)

	state := tunnelState{PID: os.Getpid(), Phase: phaseStarting}
	fail := func(err error) error {
		log.Printf("falhou: %v", err)
		state.Phase, state.Error = phaseFailed, err.Error()
		writeTunnelState(dir, state)
		return err
	}

	if !isElevated() {
		return fail(errors.New("o processo do tunel precisa rodar como administrador"))
	}

	retireOldHelper(dir)
	os.Remove(stopPath(dir))
	if err := writeTunnelState(dir, state); err != nil {
		log.Printf("nao consegui gravar o estado: %v", err)
	}

	state.ConfHash, _ = confHash(confPath)
	cfg, err := loadConf(confPath)
	if err != nil {
		return fail(err)
	}
	if err := cfg.ResolveEndpoints(); err != nil {
		return fail(fmt.Errorf("nao consegui resolver o endereco do servidor da VPN: %v", err))
	}

	routes := TunnelRoutes(ResolveDiscordHosts(5 * time.Second))
	if err := checkEndpoints(cfg, routes); err != nil {
		return fail(err)
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
		return fail(fmt.Errorf("nao consegui criar o adaptador de rede do tunel: %v", err))
	}
	closed := false
	closeAdapter := func() {
		if !closed {
			closed = true
			adapter.Close()
		}
	}
	defer closeAdapter()

	if running, err := driver.RunningVersion(); err == nil {
		log.Printf("WireGuardNT %d.%d", (running>>16)&0xffff, running&0xffff)
	}
	if err := adapter.SetLogging(driver.AdapterLogOn); err != nil {
		log.Printf("sem log do driver: %v", err)
	}
	// O aviso entra antes de o adaptador subir: registrado depois, uma interface IPv6 que
	// aparecesse no meio do caminho ficaria sem rota, e o Discord sairia por ela direto.
	network := &tunnelNetwork{luid: adapter.LUID(), cfg: cfg, routes: routes}
	callback, err := winipcfg.RegisterInterfaceChangeCallback(func(kind winipcfg.MibNotificationType, iface *winipcfg.MibIPInterfaceRow) {
		if kind != winipcfg.MibAddInstance || iface.InterfaceLUID != network.luid {
			return
		}
		family := iface.Family
		go func() {
			if err := network.configureFamily(family); err != nil {
				log.Printf("reconfigurando a familia %d: %v", family, err)
			}
		}()
	})
	if err != nil {
		log.Printf("sem aviso de mudanca de interface: %v", err)
	} else {
		defer callback.Unregister()
	}

	if err := adapter.SetConfiguration(cfg.ToDriverConfiguration()); err != nil {
		return fail(fmt.Errorf("o driver recusou a configuracao da VPN: %v", err))
	}
	if err := adapter.SetAdapterState(driver.AdapterStateUp); err != nil {
		return fail(fmt.Errorf("nao consegui ligar o adaptador do tunel: %v", err))
	}
	if err := network.configure(); err != nil {
		return fail(err)
	}

	summary := summarize(cfg)
	state.Phase = phaseReady
	state.Address = summary.Address
	state.Endpoint = summary.Endpoint
	state.Routes = len(routes)
	if err := writeTunnelState(dir, state); err != nil {
		log.Printf("nao consegui gravar o estado: %v", err)
	}
	log.Printf("tunel pronto: %d rotas, servidor %s", len(routes), summary.Endpoint)

	reason := watchDiscord(dir, watch, adapter, cfg)
	log.Printf("encerrando: %s", reason)

	closeAdapter()
	// O driver só é removido se nenhum outro adaptador WireGuard estiver em uso, então um
	// app de VPN que dependa dele continua de pé. Sem isso o Windows ficaria com o driver
	// instalado depois de um programa que promete não instalar nada.
	if err := driver.Uninstall(); err != nil {
		log.Printf("driver mantido: %v", err)
	}
	state.Phase, state.Error = phaseStopped, ""
	writeTunnelState(dir, state)
	return nil
}

func watchDiscord(dir, watch string, adapter *driver.Adapter, cfg *conf.Config) string {
	started := time.Now()
	lastReport := started
	var lastSeen time.Time

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for now := range ticker.C {
		if _, err := os.Stat(stopPath(dir)); err == nil {
			os.Remove(stopPath(dir))
			return "o launcher pediu para desligar"
		}

		if (watch != "" && processRunning(watch)) || holdActive(dir) {
			lastSeen = now
		}
		switch {
		case lastSeen.IsZero() && now.Sub(started) > discordStartGrace:
			return "o Discord nao chegou a abrir"
		case !lastSeen.IsZero() && now.Sub(lastSeen) > discordAbsenceGrace:
			return "o Discord foi fechado"
		}

		if adapterState, err := adapter.AdapterState(); err == nil && adapterState == driver.AdapterStateDown {
			log.Printf("o adaptador caiu, religando")
			if err := adapter.SetConfiguration(cfg.ToDriverConfiguration()); err != nil {
				log.Printf("religar, configuracao: %v", err)
			}
			if err := adapter.SetAdapterState(driver.AdapterStateUp); err != nil {
				log.Printf("religar, estado: %v", err)
			}
		}

		if now.Sub(lastReport) >= time.Minute {
			lastReport = now
			log.Printf("handshake %s", handshakeAge(adapter))
		}
	}
	return ""
}
