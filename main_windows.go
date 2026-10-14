//go:build windows

package main

import (
	"flag"
	"io"
	"os"

	"golang.org/x/sys/windows"
)

// Um executável só faz quatro papéis: a janela, que é o padrão, e três modos internos que
// a própria janela chama. Opções desconhecidas, como o -force de um atalho da 2.0, não
// impedem nada: a janela abre do mesmo jeito.
func main() {
	flags := flag.NewFlagSet("DiscordGoLiveBypass", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	service := flags.Bool("service", false, "")
	install := flags.Bool("install", false, "")
	uninstall := flags.Bool("uninstall", false, "")
	owner := flags.String("owner", "", "")
	result := flags.String("result", "", "")
	waitPID := flags.Int("wait-pid", 0, "")
	if err := flags.Parse(os.Args[1:]); err != nil {
		*service, *install, *uninstall = false, false, false
	}

	switch {
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
