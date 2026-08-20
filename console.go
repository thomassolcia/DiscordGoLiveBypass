package main

import "os"

// Saida redirecionada para arquivo ou pipe nao e console: spinner e cor viram sujeira
// no meio do texto, entao a UI cai para linhas simples.
func charDevice(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
