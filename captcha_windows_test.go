//go:build windows

package main

import (
	"net/url"
	"os"
	"testing"
)

func TestValidCaptchaToken(t *testing.T) {
	for token, want := range map[string]bool{
		"desafio:resposta": true,
		"desafio:":         false,
		"outro:resposta":   false,
		"desafioresposta":  false,
		"":                 false,
	} {
		if got := validCaptchaToken(token, "desafio"); got != want {
			t.Errorf("validCaptchaToken(%q) = %v, queria %v", token, got, want)
		}
	}
	if validCaptchaToken("x:y", "") {
		t.Error("sem desafio nenhum token deveria passar")
	}
}

// Abre a janela de verdade com uma página que imita o CAPTCHA do Proton: ela manda o
// token como objeto, igual à página real. Confere a ponte que transforma o objeto em
// texto, a validação do desafio e o fechamento sozinho. DGLB_JANELA=1 go test -run Verificacao.
func TestJanelaVerificacaoAoVivo(t *testing.T) {
	if os.Getenv("DGLB_JANELA") == "" {
		t.Skip("defina DGLB_JANELA=1 para abrir a janela de verificação de teste")
	}
	page := "data:text/html;charset=utf-8," + url.PathEscape(`<!doctype html><title>teste</title><p>verificando</p>
<script>
setTimeout(() => chrome.webview.postMessage({type: 'pm_height', height: 400}), 200);
setTimeout(() => chrome.webview.postMessage({type: 'pm_captcha', token: 'outro-desafio:x'}), 400);
setTimeout(() => chrome.webview.postMessage({type: 'pm_captcha', token: 'desafio-teste:resposta-ok'}), 700);
</script>`)

	token, err := runCaptchaWindow(page, "desafio-teste")
	if err != nil {
		t.Fatalf("runCaptchaWindow: %v", err)
	}
	if token != "desafio-teste:resposta-ok" {
		t.Errorf("token = %q", token)
	}
}
