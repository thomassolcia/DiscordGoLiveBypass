# Contribuindo

Obrigado por querer ajudar. Este guia explica como o projeto é organizado e o que uma mudança precisa ter para entrar sem muita ida e volta.

## Antes de começar

Para bug, abra uma issue antes do pull request, com a mensagem que apareceu na janela e a saída do `Validar-rota.ps1` rodado com o Discord aberto. Ele não mostra a chave da VPN, mas confira antes de colar: o IP do servidor aparece lá e você pode preferir apagá-lo.

Para mudança grande, como uma nova forma de levar o tráfego para fora ou suporte a outro sistema, abra a issue primeiro também. É mais barato discutir a ideia do que reescrever um pull request pronto.

Nunca anexe o seu arquivo `.conf` nem cole a linha `PrivateKey` dele em issue, pull request ou commit. Quem tem essa linha usa a sua conta da VPN.

## Rodando o projeto

Precisa do [Go](https://go.dev/dl/) e de Windows. Os testes rodam sem rede, sem administrador e sem Discord instalado:

```
go vet ./...
go test ./...
```

Para gerar o executável igual ao da release, com o driver WireGuardNT embutido:

```
.\Compilar.ps1
```

Testar o túnel de verdade pede três coisas que a CI não tem: administrador, um Discord instalado e um arquivo WireGuard seu. Se a sua mudança mexe no serviço, no túnel ou nas rotas (`service_windows.go`, `engine_windows.go`, `state.go`, `routes.go`), rode o programa compilado, ative, abra uma call e diga no pull request o que você testou: assistir a uma transmissão, transmitir, desligar e ligar o túnel pela janela. Se mexe na janela (`gui_windows.go`, `tray_windows.go` ou `frontend/`), confira os dois temas e a bandeja. O `Validar-rota.ps1` ajuda a mostrar que o tráfego saiu pelo adaptador certo.

## Como o código é escrito

- Mensagens que aparecem no console vão sem acento, porque muita máquina ainda abre o console numa codepage antiga. Comentários e documentação podem ter acento.
- Comentário só quando explica o porquê de algo que o código não deixa claro. Comentar o que a linha faz não ajuda quem lê.
- Mensagem de erro diz o que aconteceu e o que a pessoa pode fazer. Se ela for nova, ganha uma entrada no FAQ do README com o texto que aparece na tela.
- Nenhuma chave em log ou mensagem, nem quando o erro vem de uma biblioteca: o parser oficial do WireGuard repete a chave malformada no erro, e o `redactKeys` existe por isso.
- Sem dependência nova sem necessidade. Hoje só entram o `x/sys` e as bibliotecas oficiais do WireGuard.
- Rode `gofmt` antes do commit.

## Commits

O projeto segue o [padrão de commits do iuricode](https://github.com/iuricode/padroes-de-commits), no formato `<emoji> tipo: descrição`, com o emoji como caractere e a descrição em português, minúscula, curta e sem ponto final:

```
✨ feat: primeira abertura guiada
🐛 fix: chave fora da mensagem de erro
📚 docs: faq do erro 2012
```

Os mais usados aqui:

| emoji | tipo | quando |
| --- | --- | --- |
| ✨ | `feat` | funcionalidade nova |
| 🐛 | `fix` | correção de bug |
| 📚 | `docs` | README, este guia |
| 🧪 | `test` | testes |
| ♻️ | `refactor` | mudança sem alterar comportamento |
| 🔧 | `chore` | configuração e scripts |
| 🧱 | `ci` | workflows |
| 🗑️ | `remove` | remoção de arquivo ou funcionalidade |

Um commit por mudança lógica. Corpo de commit só quando a motivação não aparece no diff.

## Pull request

1. Crie uma branch a partir da `main`.
2. Mantenha o pull request focado em um assunto. Dois consertos independentes viram dois pull requests.
3. A CI precisa passar: ela roda `go vet`, `go test` e o `Compilar.ps1`.
4. Na descrição, conte o que mudou, por quê e como você testou. Se não deu para testar com o Discord de verdade, diga isso também.

## A janela

A página fica em `frontend/`, em HTML, CSS e JavaScript sem framework e sem etapa de build, para o projeto continuar compilando com um comando só. Cada método exportado do `App`, em `gui_windows.go`, aparece na página como `window.go.main.App.<Método>`. As cores vêm das variáveis no começo do `style.css`, uma paleta para cada tema: cor nova entra nas duas.
