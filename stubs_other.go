//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// O programa depende do driver WireGuard e do serviço do Windows. Fora dele o código só
// compila, para os testes das partes portáveis rodarem em qualquer máquina.

func processRunning(string) bool { return false }
func terminateAll(string) error  { return nil }

func downloadsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Downloads")
}

func main() {
	fmt.Fprintln(os.Stderr, "DiscordGoLiveBypass só funciona no Windows.")
	os.Exit(1)
}
