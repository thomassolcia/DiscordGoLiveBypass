//go:build !windows

package main

import (
	"errors"
	"os"
	"path/filepath"
)

// O túnel depende do driver WireGuard do Windows. Fora dele o código só compila, para
// os testes das partes portáveis rodarem em qualquer máquina.
var errWindowsOnly = errors.New("o tunel so funciona no Windows")

func isElevated() bool                             { return false }
func processAlive(int) bool                        { return false }
func helperAlive(int) bool                         { return false }
func startTunnelHelper([]string) (int, error)      { return 0, errWindowsOnly }
func launchUnelevated(string) error                { return errWindowsOnly }
func openBrowser(string) error                     { return errWindowsOnly }
func runTunnelHelper(string, string, string) error { return errWindowsOnly }
func ValidateConf(string) (confSummary, error)     { return confSummary{}, errWindowsOnly }

func downloadsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Downloads")
}
