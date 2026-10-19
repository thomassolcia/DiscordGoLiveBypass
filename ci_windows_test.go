//go:build windows

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	commit := func(mensagem string) string {
		t.Helper()
		git("add", "-A")
		git("commit", "-q", "-m", mensagem)
		return git("rev-parse", "HEAD")
	}

	git("init", "-q")
	escrever("main.go", "package main\n")
	escrever("README.md", "# teste\n")
	escrever("docs/janela.png", "png")
	inicio := commit("inicio")

	type caso struct{ nome, commit, base, quer string }
	var casos []caso

	mudancas := []struct {
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
	for _, m := range mudancas {
		git("checkout", "-q", "--detach", inicio)
		m.mudar()
		casos = append(casos, caso{m.nome, commit(m.nome), inicio, m.quer})
	}

	// workflow_dispatch não tem base, e o push que cria uma branch manda zeros no lugar do
	// commit anterior. Nos dois casos não dá para saber o que mudou.
	casos = append(casos,
		caso{"sem base", inicio, "", "codigo=true"},
		caso{"base que o clone não tem", inicio, strings.Repeat("0", 40), "codigo=true"},
	)

	// No pull_request o ci.yml compara o merge do PR com o primeiro pai. Código que entrou na
	// main depois que a branch saiu não é do PR e não pode fazer o PR de documentação rodar.
	git("checkout", "-q", "--detach", inicio)
	escrever("README.md", "# do pr\n")
	pr := commit("pr")
	git("checkout", "-q", "--detach", inicio)
	escrever("main.go", "package main\n\nfunc g() {}\n")
	commit("main andou")
	git("merge", "-q", "--no-ff", "--no-edit", pr)
	casos = append(casos, caso{"merge de PR só com documentação", git("rev-parse", "HEAD"), "HEAD^1", "codigo=false"})

	// Todos os casos num shell só, porque cada partida do pwsh custa meio segundo e este teste
	// roda em toda execução do CI. O script é chamado com & e com o ErrorActionPreference do
	// passo do Actions, como no ci.yml. O GITHUB_OUTPUT herdado do passo de testes é trocado
	// antes de cada chamada, para o script não escrever nele.
	aspas := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	saidas := t.TempDir()
	var roteiro strings.Builder
	roteiro.WriteString("$ErrorActionPreference = 'stop'\n")
	for i, c := range casos {
		fmt.Fprintf(&roteiro, "$env:GITHUB_OUTPUT = %s\n", aspas(filepath.Join(saidas, strconv.Itoa(i))))
		fmt.Fprintf(&roteiro, "git checkout -q --detach %s; if ($LASTEXITCODE) { exit 1 }\n", c.commit)
		fmt.Fprintf(&roteiro, "& %s -Base %s\n", aspas(script), aspas(c.base))
	}
	arquivo := filepath.Join(t.TempDir(), "casos.ps1")
	if err := os.WriteFile(arquivo, []byte(roteiro.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(shell, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", arquivo)
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v\n%s", script, err, out)
	}

	for i, c := range casos {
		resposta, err := os.ReadFile(filepath.Join(saidas, strconv.Itoa(i)))
		if err != nil {
			t.Errorf("%s: o script não gravou a resposta no GITHUB_OUTPUT: %v", c.nome, err)
			continue
		}
		if got := strings.TrimSpace(string(resposta)); got != c.quer {
			t.Errorf("%s: %s, queria %s", c.nome, got, c.quer)
		}
	}
}
