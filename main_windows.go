//go:build windows

package main

import (
	"errors"
	"flag"
	"io"
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

// Um executável só, com vários papéis: a janela, que é o padrão, e os modos internos que
// ela chama, o serviço, a instalação, a remoção e a verificação do Proton. Opções
// desconhecidas, como o -force de um atalho da 2.0, não impedem nada: a janela abre do
// mesmo jeito.
func main() {
	flags := flag.NewFlagSet("DiscordGoLiveBypass", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	service := flags.Bool("service", false, "")
	install := flags.Bool("install", false, "")
	uninstall := flags.Bool("uninstall", false, "")
	owner := flags.String("owner", "", "")
	result := flags.String("result", "", "")
	waitPID := flags.Int("wait-pid", 0, "")
	captcha := flags.String("captcha", "", "")
	challenge := flags.String("challenge", "", "")
	if err := flags.Parse(os.Args[1:]); err != nil {
		*service, *install, *uninstall, *captcha = false, false, false, ""
	}

	switch {
	case *captcha != "":
		// Só abre a página de verificação do próprio Proton, nunca um endereço qualquer.
		if !strings.HasPrefix(*captcha, protonAPI+"/core/v4/captcha?") {
			os.Exit(writeCaptchaResult(*result, "", errors.New("endereço de verificação inesperado")))
		}
		token, err := runCaptchaWindow(*captcha, *challenge)
		os.Exit(writeCaptchaResult(*result, token, err))
	case *service:
		if err := runService(); err != nil {
			os.Exit(1)
		}
	case *install:
		os.Exit(writeResult(*result, installService(*owner)))
	case *uninstall:
		err := uninstallService(*waitPID)
		if err != nil {
			messageBox("DiscordGoLiveBypass", "Não consegui remover tudo: "+err.Error(), windows.MB_ICONERROR)
		} else {
			messageBox("DiscordGoLiveBypass", "O DiscordGoLiveBypass foi removido. O Discord volta a sair pela sua conexão normal.", windows.MB_ICONINFORMATION)
		}
		os.Exit(writeResult(*result, err))
	default:
		if err := runGUI(); err != nil {
			messageBox("DiscordGoLiveBypass", "A janela não abriu: "+err.Error(), windows.MB_ICONERROR)
			os.Exit(1)
		}
	}
}
