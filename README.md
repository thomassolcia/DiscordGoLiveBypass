# DiscordGoLiveBypass

Abre o Discord por um IP fora do Brasil, devolvendo o Go Live, a câmera e a transmissão de tela.

É um programa só, sem instalação, sem plugin e sem tocar nos arquivos do Discord. Você clica, ele procura um servidor no exterior, confere se funciona e abre o Discord já conectado por ele.

> **Windows apenas.** Não é VPN: só o Discord muda de rota, o resto do computador continua igual. Também não é ferramenta de privacidade: o tráfego passa por um servidor público de terceiros.

## Por que o Go Live parou de funcionar

Em 17 de agosto de 2026, a ANPD determinou, em [medida preventiva](https://www.gov.br/anpd/pt-br/assuntos/noticias/em-medida-preventiva-anpd-determina-que-discord-suspenda-transmissoes-ao-vivo-no-brasil), que o Discord suspendesse transmissão de tela e vídeo no Brasil. A decisão veio depois de um caso grave, envolvendo a morte de uma adolescente alvo de uma campanha coordenada de assédio em várias plataformas. O Discord cumpriu a ordem e publicou uma [carta à comunidade brasileira](https://discord.com/blog/a-letter-to-the-discord-community-in-brazil) dizendo que trabalha para restaurar os recursos.

Na prática, para quem está no Brasil:

| parou | continua |
| --- | --- |
| Go Live, transmissão de tela, chamadas de vídeo | mensagens, servidores, canais de voz sem vídeo |

O bloqueio é aplicado por região, e é por isso que sair por um IP de fora o desfaz. Este projeto não opina sobre a decisão da ANPD nem sobre o caso que a motivou: ele muda a rota da sua conexão, como uma VPN faria.

## Como usar

1. Baixe o `.zip` na aba [Releases](../../releases) e extraia para uma pasta sua.
2. Feche o Discord por inteiro. Não basta fechar a janela: clique com o botão direito no ícone dele perto do relógio e escolha **Quit Discord**.
3. Clique duas vezes em `Abrir-Discord.bat`.

Se houver mais de um Discord na máquina, o normal e o PTB, ou o Canary junto, o `.bat` pergunta qual deles abrir antes de começar. Com um só instalado ele não pergunta nada e vai direto.

Uma janela preta mostra o progresso e fecha sozinha quando termina:

```
 ╭────────────────────────────────────────────────────────╮
 │  DiscordGoLiveBypass                                   │
 │  abre o Discord por um IP de fora, sem VPN no sistema  │
 ╰────────────────────────────────────────────────────────╯

 ▸ 1/4  Procurando a instalacao do Discord
   ✔  Discord encontrado
      C:\Users\voce\AppData\Local\Discord\app-1.0.9253\Discord.exe

 ▸ 2/4  Procurando uma saida fora de Brasil (BR)
   ⠹  ██████████░░░░░░░░░░░░░░  18/40  3 responderam ate agora

 ▸ 3/4  Confirmando por onde a conexao vai sair
   ✔  a saida esta em Alemanha (DE)

 ▸ 4/4  Abrindo o Discord
   ✔  Discord aberto, pid 14240

 ╭─ tudo pronto ──────────────────────────────────╮
 │  saida     Alemanha (DE)                       │
 │  servidor  socks5://213.136.92.91:1080         │
 │  resposta  1542 ms                             │
 │  discord   Discord, pid 14240                  │
 │  tempo     14s                                 │
 ╰────────────────────────────────────────────────╯
```

A barra da etapa 2 é a parte demorada: cada quadradinho é um servidor sendo testado contra o Discord de verdade. Serve qualquer país que não seja o Brasil. **A primeira tela demora mais que o normal**, às vezes meio minuto, porque tudo está dando a volta pelo exterior. Depois que carrega, o uso é igual ao de sempre.

Se a janela ficar aberta com uma mensagem, algo falhou: procure a mensagem no [FAQ](#faq).

**Atalho na área de trabalho:** clique com o botão direito em `Criar-atalho.ps1` e escolha **Executar com o PowerShell**. Use o atalho criado no lugar do atalho normal do Discord.

## Antes de confiar no arquivo

O Windows vai avisar que o programa não tem assinatura digital, e desconfiar de `.exe` baixado da internet é o instinto certo.

O binário não é montado na máquina de ninguém: o GitHub Actions compila a partir deste código, com log público na aba **Actions**, e cada release traz um `SHA256SUMS.txt` gerado no mesmo build. Para conferir o que você baixou:

```
Get-FileHash DiscordGoLiveBypass.exe -Algorithm SHA256
```

Se não bater com a linha correspondente do `SHA256SUMS.txt`, apague o arquivo. E quem preferir não confiar em binário nenhum pode [compilar](#compilar-do-código-fonte), que são dois comandos.

Ao abrir, o programa faz uma consulta à API pública do GitHub só para saber se existe uma versão mais nova, e avisa na última linha quando existe. Ele nunca baixa nem troca nada sozinho: a atualização continua sendo você quem faz, baixando o `.zip` novo. Nada é enviado nessa consulta, nem identificador nem informação da máquina, e `-no-update` desliga.

## FAQ

### No dia a dia

<details>
<summary><strong>Preciso rodar toda vez que for usar o Discord?</strong></summary>
<br>

Sim. A configuração vale só para aquela abertura. Se você abrir pelo atalho antigo, o Discord volta ao normal, sem bypass.
</details>

<details>
<summary><strong>Preciso deixar a janela preta aberta?</strong></summary>
<br>

Não. Ela fecha sozinha assim que o Discord sobe.
</details>

<details>
<summary><strong>Vai ficar lento?</strong></summary>
<br>

As mensagens e o carregamento inicial, um pouco. A transmissão em si não: vídeo, voz e imagens vão direto, sem desvio, justamente para não perder qualidade.
</details>

<details>
<summary><strong>Funciona no Discord estável, PTB e Canary?</strong></summary>
<br>

Nos três, e ele detecta sozinho qual está instalado. Se você tem mais de um, escolha com `-channel ptb` ou `-channel canary`.
</details>

<details>
<summary><strong>Isso é uma VPN?</strong></summary>
<br>

Não. Só o Discord muda de rota. Navegador, jogos e downloads continuam saindo pela sua conexão normal.
</details>

### Quando dá errado

<details>
<summary><strong>Apareceu "nao achei nenhum Discord instalado"</strong></summary>
<br>

O programa procurou nas pastas padrão e não encontrou, o que acontece quando o Discord foi instalado num lugar diferente do normal. Para descobrir o caminho: abra o Discord, aperte `Ctrl+Shift+Esc`, ache o processo, clique com o botão direito e escolha **Abrir local do arquivo**. Depois rode apontando para ele:

```
DiscordGoLiveBypass.exe -force -exe "C:\caminho\completo\Discord.exe"
```
</details>

<details>
<summary><strong>Apareceu "nenhuma proxy da lista passou nos testes"</strong></summary>
<br>

Todos os servidores testados estavam fora do ar ou lentos demais. É comum, eles são gratuitos e caem o tempo todo. Tente de novo, que na segunda ou terceira vez costuma funcionar. Se insistir em falhar, dê mais tempo de busca:

```
DiscordGoLiveBypass.exe -force -timeout 5m
```
</details>

<details>
<summary><strong>Apareceu "o Discord ja esta aberto"</strong></summary>
<br>

Só acontece se você chamar o programa sem o `-force`. O `Abrir-Discord.bat` e o atalho da área de trabalho já usam, então feche o Discord ou use um dos dois.
</details>

<details>
<summary><strong>O Discord abre mas fica travado em "Connecting"</strong></summary>
<br>

O servidor escolhido morreu entre o teste e a abertura, o que acontece com servidor gratuito. Feche o Discord e clique no atalho de novo: ele procura outro. O programa recusa de propósito cair para conexão direta, porque isso faria o bloqueio voltar sem avisar você.
</details>

<details>
<summary><strong>O Go Live continua bloqueado</strong></summary>
<br>

Duas possibilidades. A primeira é você ter um plugin de bypass ativo no Vencord ou BetterDiscord: ele mexe na conexão depois que o Discord abre e desfaz o que este programa fez, então desative e tente de novo. A segunda é o servidor sorteado estar num país que também tem restrição, e aí vale recusar mais países com `-exclude BR,AR,CL`.
</details>

### Segurança e risco

<details>
<summary><strong>É seguro?</strong></summary>
<br>

O tráfego de mensagens passa por um servidor público de terceiros, o que não é um risco diferente do de qualquer proxy gratuito, mas também não é zero. Se você tem o Tor instalado e aberto, o programa prefere ele automaticamente, e aí é bem mais confiável.
</details>

<details>
<summary><strong>Vou tomar ban?</strong></summary>
<br>

O programa muda a rota da sua conexão, não modifica o Discord nem automatiza nada. É o mesmo que usar uma VPN, coisa que muita gente faz sem problema. Ainda assim não existe garantia, e a decisão é sua.
</details>

<details>
<summary><strong>Meu antivírus reclamou</strong></summary>
<br>

Programa novo, sem assinatura digital, que abre conexões de rede e fecha outro programa: é o retrato do que antivírus desconfia. O código está todo aqui, o binário sai do GitHub Actions com log público e cada release traz as somas SHA256.
</details>

## Opções para quem quer ajustar

Abra o Prompt de Comando na pasta do programa e use:

| opção | para que serve |
| --- | --- |
| `-force` | fecha um Discord já aberto antes de subir o novo |
| `-check` | só testa e mostra o resultado, sem abrir o Discord |
| `-exclude BR,AR` | países recusados na saída (padrão: só `BR`) |
| `-channel canary` | força um canal: `auto`, `stable`, `ptb` ou `canary` |
| `-exe "C:\..."` | aponta o executável na mão |
| `-proxy socks5://ip:porta` | usa um servidor específico e nenhum outro |
| `-timeout 5m` | quanto tempo pode gastar procurando |
| `-fallback` | deixa cair para conexão direta se o proxy falhar |
| `-no-cache` / `-no-tor` | ignora o servidor guardado / não procura Tor local |
| `-no-update` | não consulta o GitHub atrás de versão nova |

> **Cuidado com o `-fallback`:** com ele, um servidor que morre no meio do uso vira conexão direta sem aviso, e o bloqueio volta em silêncio. Por isso não é o padrão.

## Como funciona por dentro

O Discord é um app Electron, ou seja, um Chromium disfarçado, e o Chromium aceita um parâmetro de linha de comando dizendo por onde sair para a internet. O programa aproveita isso.

Ele acha o `Discord.exe` da versão mais nova em `%LOCALAPPDATA%`, procura um servidor de saída (o que você passou em `-proxy`, o que funcionou da última vez, um Tor local aberto ou a lista pública da proxyscrape, nessa ordem) e abre o app com `--proxy-server=socks5://ip:porta`.

A validação de cada candidato vai até o fim: túnel SOCKS5, TLS e resposta `200` da API do Discord, e só então o país de saída, conferido em `ifconfig.co` por dentro do mesmo túnel. Servidor que apenas aceita a conexão passa em teste ingênuo e trava o app depois.

Mídia pesada (`cdn.discordapp.com`, `*.discordapp.net`, `*.discord.media`) sai por fora do proxy de propósito: mandá-la por um servidor gratuito destruiria a qualidade da transmissão sem ganho nenhum, já que quem determina a região é a API.

Que o mecanismo funciona dá para verificar: subindo o Discord com um servidor propositalmente morto, o carregamento inteiro morre junto, com `ERR_PROXY_CONNECTION_FAILED`, e o app não passa da tela inicial.

## Compilar do código-fonte

Precisa do [Go](https://go.dev/dl/). Nenhuma outra dependência, só a biblioteca padrão.

```
go build -o DiscordGoLiveBypass.exe .
```

| arquivo | conteúdo |
| --- | --- |
| `main.go` | opções, ordem das fontes de servidor, orquestração |
| `proxy.go` | túnel SOCKS5 e HTTP CONNECT, teste via TLS, país de saída |
| `source.go` | lista pública, ranqueamento, testes em paralelo, Tor, cache |
| `discord.go` | detecção da instalação, instância única, abertura |
| `ui.go` | etapas, spinner, barra de progresso, caixas de resumo e de erro |
| `update.go` | versão do build e consulta da última release no GitHub |
| `console*.go` | liga ANSI e UTF-8 no console, e detecta quando cair para texto puro |

## Créditos

A lógica de busca, ranqueamento e validação de servidores é adaptada do **GoLiveBypass**, plugin Vencord que resolve o mesmo problema por dentro do Discord. Este projeto faz por fora, sem tocar na instalação. Por isso a licença aqui é a mesma dele, GPL-3.0.
