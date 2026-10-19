//go:build windows

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// O ci.yml pula vet, testes e compilação quando o .github/mudou-codigo.ps1 diz que só mudou
// documentação. Errar para esse lado deixa código entrar na main sem passar por nada, então
// os casos de borda ficam aqui, rodando o script de verdade num repositório de teste.
func TestMudouCodigo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git fora do PATH")
	}
	// O Actions roda o script no pwsh. Sem ele, o powershell.exe que vem com o Windows serve.
	shell := "pwsh"
	if _, err := exec.LookPath(shell); err != nil {
		shell = "powershell"
	}
	script, err := filepath.Abs(filepath.Join(".github", "mudou-codigo.ps1"))
	if err != nil {
		t.Fatal(err)
	}

	repo := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		args = append([]string{"-c", "user.name=teste", "-c", "user.email=teste@exemplo.invalid", "-c", "commit.gpgsign=false"}, args...)
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
		}
		return strings.TrimSpace(string(out))
	}
	escrever := func(caminho, conteudo string) {
		t.Helper()
		caminho = filepath.Join(repo, filepath.FromSlash(caminho))
		if err := os.MkdirAll(filepath.Dir(caminho), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(caminho, []byte(conteudo), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	commit := func(mensagem string) {
		t.Helper()
		git("add", "-A")
		git("commit", "-q", "-m", mensagem)
	}
	// Dentro do Actions o go test herda o GITHUB_OUTPUT do passo, e o script não pode escrever
	// nele: cada execução ganha o seu, que o exec.Cmd põe por cima do herdado.
	rodar := func(base string) string {
		t.Helper()
		args := []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script}
		if base != "" {
			args = append(args, "-Base", base)
		}
		saida := filepath.Join(t.TempDir(), "saida")
		cmd := exec.Command(shell, args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GITHUB_OUTPUT="+saida)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s -Base %q: %v\n%s", script, base, err, out)
		}
		resposta, err := os.ReadFile(saida)
		if err != nil {
			t.Fatalf("o script não gravou a resposta no GITHUB_OUTPUT: %v", err)
		}
		return strings.TrimSpace(string(resposta))
	}

	git("init", "-q")
	escrever("main.go", "package main\n")
	escrever("README.md", "# teste\n")
	escrever("docs/janela.png", "png")
	commit("inicio")
	inicio := git("rev-parse", "HEAD")

	casos := []struct {
		nome  string
		mudar func()
		quer  string
	}{
		{"só o README", func() { escrever("README.md", "# outro\n") }, "codigo=false"},
		{"notas de versão e capturas", func() {
			escrever(".github/notas-de-versao/v9.9.9.md", "notas\n")
			escrever("docs/janela.png", "png novo")
			escrever("CONTRIBUTING.md", "guia\n")
		}, "codigo=false"},
		{"código junto com documentação", func() {
			escrever("main.go", "package main\n\nfunc f() {}\n")
			escrever("README.md", "# outro\n")
		}, "codigo=true"},
		{"workflow", func() { escrever(".github/workflows/ci.yml", "name: ci\n") }, "codigo=true"},
		{"código com nome de documentação", func() { escrever("docs.go", "package main\n") }, "codigo=true"},
		{"código que começa com o nome do README", func() { escrever("README.md.go", "package main\n") }, "codigo=true"},
		// O frontend/ inteiro vai embutido no binário, README incluído.
		{"README dentro do frontend/", func() { escrever("frontend/README.md", "notas\n") }, "codigo=true"},
		{"código apagado", func() { git("rm", "-q", "main.go") }, "codigo=true"},
		// Com a detecção de renomeação do git, só o docs/main.go apareceria na lista.
		{"código movido para docs/", func() { git("mv", "main.go", "docs/main.go") }, "codigo=true"},
	}
	for _, c := range casos {
		git("checkout", "-q", "--detach", inicio)
		c.mudar()
		commit(c.nome)
		if got := rodar(inicio); got != c.quer {
			t.Errorf("%s: %s, queria %s", c.nome, got, c.quer)
		}
	}

	// workflow_dispatch não tem base, e o push que cria uma branch manda zeros no lugar do
	// commit anterior. Nos dois casos não dá para saber o que mudou.
	for _, base := range []string{"", strings.Repeat("0", 40)} {
		if got := rodar(base); got != "codigo=true" {
			t.Errorf("base %q: %s, queria codigo=true", base, got)
		}
	}

	// No pull_request o ci.yml compara o merge do PR com o primeiro pai. Código que entrou na
	// main depois que a branch saiu não é do PR e não pode fazer o PR de documentação rodar.
	git("checkout", "-q", "--detach", inicio)
	escrever("README.md", "# do pr\n")
	commit("pr")
	pr := git("rev-parse", "HEAD")
	git("checkout", "-q", "--detach", inicio)
	escrever("main.go", "package main\n\nfunc g() {}\n")
	commit("main andou")
	git("merge", "-q", "--no-ff", "--no-edit", pr)
	if got := rodar("HEAD^1"); got != "codigo=false" {
		t.Errorf("merge de PR só com documentação: %s, queria codigo=false", got)
	}
}
