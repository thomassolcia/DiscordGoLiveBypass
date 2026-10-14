//go:build windows

package main

import (
	"context"
	"errors"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Caminho falso que a página usa para falar do arquivo gerado pela conta do Proton. Ele
// não existe em disco: a chave fica só na memória até ir para o serviço.
const protonConfPath = "proton://gerado"

type ProtonEntrada struct {
	Email  string `json:"email"`
	Senha  string `json:"senha"`
	Codigo string `json:"codigo"`
}

type ProtonPais struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	Melhor   string `json:"melhor"`
	Quantos  int    `json:"quantos"`
	Primeiro bool   `json:"primeiro"`
}

type ProtonResposta struct {
	Etapa  string       `json:"etapa"`
	Paises []ProtonPais `json:"paises"`
}

// Países na ordem do melhor servidor de cada um, que é a ordem em que o Proton os
// recomendaria para esta conexão.
func countriesOf(servers []protonServer) []ProtonPais {
	var list []ProtonPais
	index := map[string]int{}
	for _, server := range servers {
		if i, ok := index[server.Country]; ok {
			list[i].Quantos++
			continue
		}
		index[server.Country] = len(list)
		list = append(list, ProtonPais{Code: server.Country, Label: CountryLabel(server.Country), Melhor: server.Name, Quantos: 1})
	}
	if len(list) > 0 {
		list[0].Primeiro = true
	}
	return list
}

func (a *App) clearProton() {
	a.mu.Lock()
	client := a.proton
	a.proton, a.protonServers = nil, nil
	a.mu.Unlock()
	if client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		client.logout(ctx)
	}
}

// ProtonEntrar faz o login e, se der certo, devolve os países com servidor gratuito. A
// verificação de segurança, quando o Proton pede, abre e é refeita aqui mesmo, sem a
// página precisar saber de nada além do aviso.
func (a *App) ProtonEntrar(in ProtonEntrada) (ProtonResposta, error) {
	a.clearProton()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	client := newProtonClient()
	token := ""
	for attempt := 0; ; attempt++ {
		err := client.login(ctx, in.Email, in.Senha, in.Codigo, token)
		if err == nil {
			break
		}
		if errors.Is(err, errProtonTwoFactor) {
			return ProtonResposta{Etapa: "codigo"}, nil
		}
		var captcha *ProtonCaptcha
		if !errors.As(err, &captcha) || attempt >= 2 {
			return ProtonResposta{}, err
		}
		runtime.EventsEmit(a.ctx, "proton-verificacao")
		if token, err = solveCaptcha(ctx, captcha); err != nil {
			return ProtonResposta{}, err
		}
	}

	servers, err := client.servers(ctx)
	if err != nil {
		client.logout(ctx)
		return ProtonResposta{}, err
	}
	a.mu.Lock()
	a.proton, a.protonServers = client, servers
	a.mu.Unlock()
	return ProtonResposta{Etapa: "paises", Paises: countriesOf(servers)}, nil
}

// ProtonGerar cria o arquivo no país escolhido, ou no melhor servidor quando o país vem
// vazio, e encerra a sessão. O resultado entra na lista como qualquer arquivo importado.
func (a *App) ProtonGerar(country string) (ConfInfo, error) {
	a.mu.Lock()
	client, servers := a.proton, a.protonServers
	a.proton, a.protonServers = nil, nil
	a.mu.Unlock()
	if client == nil {
		return ConfInfo{}, errors.New("a sessão do Proton expirou, entre de novo")
	}

	server, ok := pickServer(servers, country)
	if !ok {
		client.logout(context.Background())
		return ConfInfo{}, errors.New("esse país não tem servidor gratuito disponível agora")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	text, expires, err := client.generateConf(ctx, server)
	if err != nil {
		var captcha *ProtonCaptcha
		if errors.As(err, &captcha) {
			return ConfInfo{}, errors.New("o Proton pediu outra verificação para gerar o arquivo, entre de novo")
		}
		return ConfInfo{}, err
	}

	info := ConfInfo{
		Path:     protonConfPath,
		Name:     "gerado pela sua conta do Proton",
		Server:   server.Name,
		Endpoint: server.EntryIP + ":51820",
		Modified: "válido até " + expires.Format("02/01/2006"),
		Origin:   "proton",
	}
	a.mu.Lock()
	a.protonConf, a.protonInfo = text, info
	a.mu.Unlock()
	return info, nil
}

func (a *App) ProtonCancelar() { a.clearProton() }

// O arquivo gerado só existe para ir ao serviço. Depois que ele chega lá, sai da memória.
func (a *App) protonConfText() (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.protonConf, a.protonConf != ""
}

func (a *App) forgetProtonConf() {
	a.mu.Lock()
	a.protonConf, a.protonInfo = "", ConfInfo{}
	a.mu.Unlock()
}

func (a *App) pendingProton() (ConfInfo, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.protonInfo, a.protonConf != ""
}

// A página do plano gratuito, de onde sai o cadastro. O resto do caminho é do Proton.
func (a *App) AbrirCadastroProton() { runtime.BrowserOpenURL(a.ctx, "https://protonvpn.com/free-vpn") }
