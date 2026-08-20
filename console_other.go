//go:build !windows

package main

import "os"

// Fora do Windows o terminal ja chega pronto: so o TERM=dumb precisa de cuidado.
func prepareConsole(f *os.File) (ansi bool, wide bool) {
	if !charDevice(f) || os.Getenv("TERM") == "dumb" {
		return false, false
	}
	return true, true
}
