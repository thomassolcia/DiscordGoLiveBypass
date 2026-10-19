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

Alguns testes mexem em coisas de verdade e só rodam quando pedidos por variável de ambiente: `DGLB_REDE=1` consulta o Discord e o `/auth/info` do Proton (com um usuário inventado, sem senha), `DGLB_JANELA=1` abre a janela de verificação com uma página de teste e `DGLB_BANDEJA=1` põe um ícone de teste na bandeja. Rode o que tiver a ver com a sua mudança, por exemplo `DGLB_REDE=1 go test -run AoVivo`.

Para gerar o executável igual ao da release, com o driver WireGuardNT embutido:

```
.\Compilar.ps1
```

Testar o túnel de verdade pede três coisas que a CI não tem: administrador, um Discord instalado e um arquivo WireGuard seu. Se a sua mudança mexe no serviço, no túnel ou nas rotas (`service_windows.go`, `engine_windows.go`, `state.go`, `routes.go`, `split.go`, `watch.go`, `logtail_windows.go`), rode o programa compilado, ative, abra uma call e diga no pull request o que você testou: assistir a uma transmissão, transmitir, desligar e ligar o túnel pela janela, nos dois modos. No modo **Só o Go Live**, confira também que a voz ficou num servidor perto de você e que **Transmissões no túnel** sobe quando a transmissão abre e volta a 0 quando ela fecha. Se mexe na janela (`gui_windows.go`, `tray_windows.go` ou `frontend/`), confira os dois temas e a bandeja. O `Validar-rota.ps1` ajuda a mostrar que o tráfego saiu pelo adaptador certo.

Se mexe no início com o Windows (`startup_windows.go`, o `set_autostart` do serviço ou a abertura com `-bandeja`), confira as duas pontas: o tipo de início do serviço, com `sc.exe qc DiscordGoLiveBypass`, e a entrada `DiscordGoLiveBypass` em `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`. Com a opção ligada, rode o comando gravado na entrada e veja a janela abrir escondida, só com o ícone. Com ela desligada, pare o serviço como administrador e abra a janela sem administrador: ela precisa pô-lo de pé sozinha. Se puder, reinicie o computador uma vez para ver o caminho de verdade.

O registro do Discord muda de formato sem aviso, e o `split.go` depende dele. Os testes guardam linhas reais copiadas do `renderer_js.log`; se o Discord mudar alguma, acrescente a linha nova aos testes em vez de trocar a antiga, para quem ainda está numa versão anterior do Discord continuar coberto. Ao copiar linhas do registro, apague as que trazem `local address`, que mostram o seu IP.

Se mexe no login do Proton (`proton.go`, `gui_proton_windows.go`, `captcha_windows.go`), teste com uma conta sua, gratuita serve, e conte no pull request se ela tinha verificação em duas etapas e se apareceu a verificação de robô. Os testes cobrem a conversão de chave e a montagem do arquivo, mas o Proton muda as regras do login sem avisar e só uma conta de verdade mostra isso.

## Como o código é escrito

- Mensagens que aparecem no console vão sem acento, porque muita máquina ainda abre o console numa codepage antiga. Comentários e documentação podem ter acento.
- Comentário só quando explica o porquê de algo que o código não deixa claro. Comentar o que a linha faz não ajuda quem lê.
- Mensagem de erro diz o que aconteceu e o que a pessoa pode fazer. Se ela for nova, ganha uma entrada no FAQ do README com o texto que aparece na tela.
- Nenhuma chave em log ou mensagem, nem quando o erro vem de uma biblioteca: o parser oficial do WireGuard repete a chave malformada no erro, e o `redactKeys` existe por isso.
- Nenhuma senha, token ou código de duas etapas em log, arquivo ou mensagem de erro. A sessão do Proton vive só na memória e é encerrada assim que o arquivo é gerado.
- Sem dependência nova sem necessidade. Hoje entram o `x/sys`, as bibliotecas oficiais do WireGuard, o Wails para a janela e o `go-srp` do próprio Proton para o login.
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

O ícone sai dos SVGs em `winres/`: o `logo.svg` para os tamanhos grandes, o `logo-pequeno.svg` para 32 e 24 px e o `logo-solto.svg`, sem o fundo escuro, para a bandeja. Depois de mexer num deles, rode `winres\Gerar-icones.ps1` e faça commit dos PNGs junto. O topo da janela usa o mesmo desenho do `logo-pequeno.svg`, direto no `index.html`.
