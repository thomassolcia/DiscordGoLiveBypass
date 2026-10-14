<div align="center">

# DiscordGoLiveBypass

**Abre o Discord com só ele passando por uma VPN fora do Brasil, devolvendo o Go Live, a câmera e a transmissão de tela.**

[![Versão](https://img.shields.io/github/v/release/thomassolcia/DiscordGoLiveBypass?style=flat-square&label=vers%C3%A3o&color=5865F2)](../../releases/latest)
[![Downloads](https://img.shields.io/github/downloads/thomassolcia/DiscordGoLiveBypass/total?style=flat-square&label=downloads&color=5865F2)](../../releases)
[![Build](https://img.shields.io/github/actions/workflow/status/thomassolcia/DiscordGoLiveBypass/ci.yml?style=flat-square&label=build)](../../actions)
[![Licença](https://img.shields.io/github/license/thomassolcia/DiscordGoLiveBypass?style=flat-square&label=licen%C3%A7a)](LICENSE)
![Windows](https://img.shields.io/badge/plataforma-Windows-0078D4?style=flat-square)

[**Baixar**](../../releases/latest) · [Como usar](#como-usar) · [FAQ](#faq) · [Compilar](#compilar-do-código-fonte)

</div>

É um programa só, sem instalação, sem plugin e sem tocar nos arquivos do Discord. Você clica, ele liga um túnel WireGuard que leva apenas o tráfego do Discord para fora do Brasil, confere com o próprio Discord que a saída mudou e abre o app.

> **Windows apenas.** Só o Discord muda de rota: o jogo que você está transmitindo, o navegador e o resto do computador continuam na sua conexão normal. O servidor de saída é o da sua VPN, então a confiança que vale é a que você tem nela.

## Por que o Go Live parou de funcionar

Em 17 de agosto de 2026, a ANPD determinou, em [medida preventiva](https://www.gov.br/anpd/pt-br/assuntos/noticias/em-medida-preventiva-anpd-determina-que-discord-suspenda-transmissoes-ao-vivo-no-brasil), que o Discord suspendesse transmissão de tela e vídeo no Brasil. A decisão veio depois de um caso grave, envolvendo a morte de uma adolescente alvo de uma campanha coordenada de assédio em várias plataformas. O Discord cumpriu a ordem e publicou uma [carta à comunidade brasileira](https://discord.com/blog/a-letter-to-the-discord-community-in-brazil) dizendo que trabalha para restaurar os recursos.

Na prática, para quem está no Brasil:

| parou | continua |
| --- | --- |
| Go Live, transmissão de tela, chamadas de vídeo | mensagens, servidores, canais de voz sem vídeo |

O bloqueio é aplicado por região, e é por isso que sair por um IP de fora o desfaz. Este projeto não opina sobre a decisão da ANPD nem sobre o caso que a motivou: ele muda a rota da sua conexão, como uma VPN faria.

## Por que a 2.0 trocou o proxy por um túnel

Até a 1.0.4 o programa abria o Discord com `--proxy-server`, apontando para um proxy SOCKS5 gratuito. Isso liberava os botões, mas desviava só o que o Chromium do Discord faz: o gateway, a API e o resto do HTTP. Voz e vídeo saem por outro caminho, o motor de mídia nativo, em UDP, que ignora esse parâmetro, e por isso continuavam saindo do IP brasileiro.

No começo de setembro o Discord passou a recusar essa combinação. A transmissão liberava, mas quem tentava assistir ficava no carregamento e caía em "Stream failed to start", **Erro 2012**, que é o tempo limite de 20 segundos esperando o primeiro quadro de vídeo. O áudio da live às vezes chegava, a imagem nunca. Proxy gratuito e Tor não carregam UDP, então nenhum ajuste no modelo antigo resolvia.

A 2.0 liga um túnel WireGuard e põe nele as faixas de IP do Discord: gateway, API, CDN e os servidores de voz e vídeo. Tudo o que o Discord faz sai pelo mesmo IP de fora, UDP incluso. O resto do computador não entra no túnel.

## Como usar

### Da primeira vez

O túnel precisa de um servidor WireGuard. O jeito gratuito é uma conta no Proton VPN:

1. Crie uma conta gratuita em [protonvpn.com](https://protonvpn.com).
2. Entre em [account.protonvpn.com](https://account.protonvpn.com/downloads), abra **Downloads** e vá até **WireGuard configuration**.
3. Escolha a plataforma **Windows**, um servidor gratuito e clique em **Create**.
4. Baixe o arquivo `.conf`.

Não precisa instalar o aplicativo do Proton: o arquivo basta. Qualquer outra VPN que entregue um arquivo WireGuard serve do mesmo jeito, e quem já paga uma usa o arquivo dela.

Você também pode pular esses passos e ir direto para os de baixo: sem arquivo, o programa mostra este mesmo passo a passo, abre a página do Proton se você quiser e espera o arquivo chegar na pasta Downloads.

### Toda vez

1. Baixe o `.zip` na aba [Releases](../../releases) e extraia para uma pasta sua.
2. Feche o Discord por inteiro. Não basta fechar a janela: clique com o botão direito no ícone dele perto do relógio e escolha **Quit Discord**.
3. Clique duas vezes em `Abrir-Discord.bat`. Dois cliques direto no `DiscordGoLiveBypass.exe` também funcionam; o `.bat` só acrescenta a pergunta de qual Discord abrir quando há mais de um instalado.
4. O Windows pede permissão de administrador. Responda **Sim**: é ela que deixa criar o adaptador de rede do túnel.

Na primeira vez, o programa acha o `.conf` na pasta Downloads, pergunta se pode usar e guarda uma cópia. Das próximas vezes ele usa a cópia e não pergunta mais nada.

Se houver mais de um Discord na máquina, o normal e o PTB, ou o Canary junto, o `.bat` pergunta qual deles abrir antes de começar. Com um só instalado ele não pergunta nada e vai direto.

Uma janela preta mostra as etapas e fecha sozinha quando o Discord abre. Esta é uma abertura de verdade, com o servidor gratuito dos Estados Unidos e o arquivo já guardado:

```
 > 1/4  Procurando a instalacao do Discord
   +  Discord Canary encontrado
      C:\Users\voce\AppData\Local\DiscordCanary\app-1.0.1227\DiscordCanary.exe
   !  o Discord Canary ja esta aberto, sera reiniciado no fim

 > 2/4  Procurando o seu arquivo de VPN
   +  usando o arquivo guardado da primeira vez
      C:\Users\voce\AppData\Local\DiscordGoLiveBypass\wireguard.conf
   +  configuracao WireGuard valida, servidor 146.70.202.34:51820

 > 3/4  Ligando o tunel so para o Discord
   -  o Windows vai pedir permissao de administrador, e ela que deixa criar o adaptador de rede do tunel
   +  tunel ligado, 5 faixas de IP do Discord passam por ele
   +  o Discord ve a saida em Estados Unidos (US), 768 ms

 > 4/4  Abrindo o Discord Canary
   +  instancia antiga encerrada
   +  Discord Canary, pid 10768 aberto

 +- tudo pronto ---------------------------------------------+
 |  saida     Estados Unidos (US)                            |
 |  ip visto  146.70.202.37                                  |
 |  servidor  146.70.202.34:51820                            |
 |  resposta  768 ms                                         |
 |  discord   Discord Canary, pid 10768                      |
 |  tunel     fica ligado enquanto o Discord estiver aberto  |
 |  tempo     7s                                             |
 +-----------------------------------------------------------+
```

O túnel fica ligado em segundo plano enquanto o Discord estiver aberto e se desliga sozinho meio minuto depois que você fecha o Discord.

Se a janela ficar aberta com uma mensagem, algo falhou: procure a mensagem no [FAQ](#faq).

**Atalho na área de trabalho:** clique com o botão direito em `Criar-atalho.ps1` e escolha **Executar com o PowerShell**. Use o atalho criado no lugar do atalho normal do Discord.

## Antes de confiar no arquivo

O Windows vai avisar que o programa não tem assinatura digital, e desconfiar de `.exe` baixado da internet é o instinto certo.

O binário não é montado na máquina de ninguém: o GitHub Actions compila a partir deste código, com log público na aba **Actions**, e cada release traz um `SHA256SUMS.txt` gerado no mesmo build. Para conferir o que você baixou:

```
Get-FileHash DiscordGoLiveBypass.exe -Algorithm SHA256
```

Se não bater com a linha correspondente do `SHA256SUMS.txt`, apague o arquivo. E quem preferir não confiar em binário nenhum pode [compilar](#compilar-do-código-fonte), com um comando só.

Dentro do executável vai o driver WireGuardNT, o mesmo que o cliente oficial do WireGuard para Windows usa, sem nenhuma modificação. Ele é baixado no build do site oficial e só entra se o hash bater com o que o próprio cliente oficial fixa, e vem assinado pela WireGuard LLC.

Ao abrir, o programa faz uma consulta à API pública do GitHub só para saber se existe uma versão mais nova, e avisa na última linha quando existe. Ele nunca baixa nem troca nada sozinho: a atualização continua sendo você quem faz, baixando o `.zip` novo. Nada é enviado nessa consulta, nem identificador nem informação da máquina, e `-no-update` desliga.

## FAQ

### No dia a dia

<details>
<summary><strong>Preciso de conta numa VPN?</strong></summary>
<br>

Precisa de um servidor WireGuard, e a conta gratuita do Proton VPN é o jeito mais simples de ter um. A conta é sua e o arquivo também: o programa não traz chave embutida, porque uma chave dividida entre milhares de pessoas derruba a conexão de todas (o WireGuard trata cada chave como um aparelho só) e ainda poria o tráfego de todo mundo na conta de uma pessoa.
</details>

<details>
<summary><strong>Por que não usar uma VPN comum direto?</strong></summary>
<br>

Dá, e funciona. A diferença é que a VPN comum leva o computador inteiro: o jogo que você está transmitindo ganha ping, e banco, streaming e downloads passam a sair pelo servidor dela. Aqui só o Discord vai pelo túnel. Se você não se importa de pôr tudo na VPN, ela resolve com o mesmo esforço.
</details>

<details>
<summary><strong>Preciso rodar toda vez que for usar o Discord?</strong></summary>
<br>

Sim. O túnel vale enquanto aquele Discord estiver aberto. Se você abrir pelo atalho antigo, o Discord volta ao normal, sem bypass.
</details>

<details>
<summary><strong>Por que pede administrador toda vez?</strong></summary>
<br>

Criar um adaptador de rede e mexer em rota é coisa que o Windows só deixa com administrador. O pedido é só para o processo do túnel: o Discord continua abrindo como o seu usuário. Se o túnel da última vez ainda estiver ligado, com o mesmo arquivo, o programa reaproveita e não pede de novo.
</details>

<details>
<summary><strong>O que fica instalado no meu computador?</strong></summary>
<br>

Enquanto o túnel está ligado, um adaptador de rede chamado `DiscordGoLiveBypass` e o driver WireGuardNT. Quando o túnel desliga, os dois saem. Na pasta `%LOCALAPPDATA%\DiscordGoLiveBypass` ficam a cópia do seu `.conf`, o estado do túnel e o registro dele, `tunnel.log`, que não grava chave nenhuma.
</details>

<details>
<summary><strong>Vai ficar lento?</strong></summary>
<br>

O Discord passa a dar a volta pelo servidor da VPN, então o ping dele sobe um pouco, mais ou menos o quanto o servidor está longe. O resto do computador não muda. O plano gratuito do Proton tem velocidade média e servidores às vezes cheios; se a chamada engasgar, gere um arquivo com outro servidor.
</details>

<details>
<summary><strong>Funciona no Discord estável, PTB e Canary?</strong></summary>
<br>

Nos três, e ele detecta sozinho qual está instalado. Se você tem mais de um, escolha com `-channel ptb` ou `-channel canary`.
</details>

<details>
<summary><strong>Quero trocar de servidor ou de VPN</strong></summary>
<br>

Baixe o arquivo novo e rode uma vez com `-conf` apontando para ele. A cópia guardada é substituída e as próximas aberturas usam o novo:

```
DiscordGoLiveBypass.exe -force -conf "C:\Users\voce\Downloads\outro-servidor.conf"
```
</details>

### Quando dá errado

<details>
<summary><strong>Apareceu "nao achei nenhum arquivo de VPN"</strong></summary>
<br>

O programa não tem um `.conf` guardado e não achou nenhum na pasta Downloads. Siga os passos de [Da primeira vez](#da-primeira-vez) e rode pelo `Abrir-Discord.bat`, que pergunta e espera o arquivo. Se ele estiver em outra pasta, aponte com `-conf`.
</details>

<details>
<summary><strong>Apareceu "nao e uma configuracao WireGuard valida"</strong></summary>
<br>

O arquivo está incompleto ou não é de WireGuard. Arquivo de OpenVPN (`.ovpn`) não serve. Gere de novo no site da VPN, na opção de WireGuard, e rode com `-conf`.
</details>

<details>
<summary><strong>Apareceu "a permissao de administrador foi negada"</strong></summary>
<br>

O aviso do Windows foi respondido com Não ou fechado. Rode de novo e responda Sim. Se a sua conta não for administradora, o Windows pede a senha de uma que seja.
</details>

<details>
<summary><strong>Apareceu "o tunel subiu, mas o Discord nao respondeu por ele"</strong></summary>
<br>

O adaptador foi criado, mas o servidor da VPN não respondeu. Costuma ser servidor fora do ar ou arquivo antigo, cuja chave a VPN já revogou. Gere outro arquivo no site da VPN e rode com `-conf`. Se continuar, rode o `Validar-rota.ps1` e anexe o resultado numa issue.
</details>

<details>
<summary><strong>Apareceu "o Discord ainda ve a saida em BR"</strong></summary>
<br>

O túnel subiu, mas o Discord continua enxergando o Brasil. Ou o servidor escolhido no arquivo fica num país recusado, e aí basta gerar outro com um servidor de fora, ou as rotas não pegaram. O `Validar-rota.ps1` mostra por qual interface cada destino do Discord está saindo.
</details>

<details>
<summary><strong>Apareceu "nao consegui criar o adaptador de rede do tunel"</strong></summary>
<br>

O Windows recusou o driver ou o adaptador. Antivírus com proteção de rede costuma ser o motivo: adicione o `DiscordGoLiveBypass.exe` às exceções. Outro app de VPN ligado ao mesmo tempo também pode atrapalhar; desligue-o e tente de novo.
</details>

<details>
<summary><strong>Apareceu "nao achei nenhum Discord instalado"</strong></summary>
<br>

O programa procurou nas pastas padrão e não encontrou, o que acontece quando o Discord foi instalado num lugar diferente do normal. Para descobrir o caminho: abra o Discord, aperte `Ctrl+Shift+Esc`, ache o processo, clique com o botão direito e escolha **Abrir local do arquivo**. Depois rode apontando para ele:

```
DiscordGoLiveBypass.exe -force -exe "C:\caminho\completo\Discord.exe"
```
</details>

<details>
<summary><strong>Apareceu "a conexao foi bloqueada pela sua propria maquina"</strong></summary>
<br>

O Windows recusou a conexão antes de ela sair para a internet, com o código `WSAEACCES` (na mensagem original: *an attempt was made to access a socket in a way forbidden by its access permissions*). Não é a VPN nem sua internet: é algo instalado aí barrando este programa, quase sempre um antivírus com proteção de rede (Avast, AVG, ESET, Kaspersky, McAfee) ou uma regra de saída do Firewall do Windows. O executável não tem assinatura digital, e programa sem assinatura é o alvo preferido dessas proteções.

O que tentar, nessa ordem:

1. Abra seu antivírus, procure a lista de exceções e adicione o `DiscordGoLiveBypass.exe`.
2. Se você usa outra VPN, feche-a e rode de novo: alguns clientes deixam um filtro de rede ativo mesmo desconectados.
3. Se continuar, confira no Firewall do Windows (Segurança Avançada, Regras de Saída) se existe uma regra bloqueando o programa.
4. Se preferir não mexer em exceção nenhuma, [compile do código-fonte](#compilar-do-código-fonte): o binário que sai do seu próprio build costuma passar.
</details>

<details>
<summary><strong>Apareceu "o Discord ja esta aberto"</strong></summary>
<br>

Só acontece quando o programa é chamado pelo terminal sem o `-force`. Dois cliques no `Abrir-Discord.bat`, no executável ou no atalho da área de trabalho já fecham o Discord aberto antes de abrir de novo.
</details>

<details>
<summary><strong>Apareceu "abriu e morreu logo em seguida"</strong></summary>
<br>

O Discord subiu e caiu antes de mostrar janela, o que quase sempre é atualização interrompida: sobra uma pasta da versão nova com o executável, mas sem os arquivos que o Chromium precisa para iniciar. Abra o Discord pelo atalho normal uma vez, deixe ele terminar de se atualizar, feche e rode de novo.

O programa já evita essas pastas sozinho e usa a última versão que estiver completa. A mensagem só aparece quando nenhuma está, ou quando você apontou uma na mão com `-exe`.
</details>

<details>
<summary><strong>O Go Live continua bloqueado ou dá Erro 2012</strong></summary>
<br>

Confira três coisas. Plugin de bypass ativo no Vencord ou BetterDiscord mexe na conexão depois que o Discord abre e pode desfazer o túnel: desative e tente de novo. Quem assiste também precisa estar com o Go Live liberado, por este programa ou por uma VPN. E o `Validar-rota.ps1`, rodado com o Discord aberto, mostra se a voz e o vídeo estão mesmo saindo pelo adaptador `DiscordGoLiveBypass`.
</details>

<details>
<summary><strong>Eu usava a 1.x, o que muda?</strong></summary>
<br>

O programa não procura mais proxy gratuito nem Tor: agora ele precisa do arquivo da sua VPN, e pede administrador para ligar o túnel. As opções `-proxy`, `-bypass`, `-route`, `-timeout`, `-no-cache`, `-no-tor` e `-fallback` deixaram de existir. Um atalho antigo que use alguma delas continua funcionando, só aparece um aviso de que ela foi ignorada.
</details>

### Segurança e risco

<details>
<summary><strong>É seguro?</strong></summary>
<br>

O tráfego do Discord passa pelo servidor da VPN que você escolheu, criptografado pelo WireGuard até lá. A chave fica só no seu computador, na cópia guardada dentro da pasta do seu usuário, em `%LOCALAPPDATA%`. O programa nunca mostra nem registra a chave, nem em mensagem de erro.
</details>

<details>
<summary><strong>Vou tomar ban?</strong></summary>
<br>

O programa muda a rota da sua conexão, não modifica o Discord nem automatiza nada. É o mesmo que usar uma VPN, coisa que muita gente faz sem problema. Ainda assim não existe garantia, e a decisão é sua.
</details>

<details>
<summary><strong>Meu antivírus reclamou</strong></summary>
<br>

Programa novo, sem assinatura digital, que cria adaptador de rede e fecha outro programa: é o retrato do que antivírus desconfia. O código está todo aqui, o binário sai do GitHub Actions com log público e cada release traz as somas SHA256.
</details>

## Opções para quem quer ajustar

Abra o Prompt de Comando na pasta do programa e use:

| opção | para que serve |
| --- | --- |
| `-force` | fecha um Discord já aberto antes de subir o novo |
| `-conf "C:\...\arquivo.conf"` | usa este arquivo WireGuard e guarda para as próximas vezes |
| `-check` | liga o túnel, mostra a saída que o Discord vê e desliga, sem abrir o Discord |
| `-exclude BR,AR` | países recusados na saída (padrão: só `BR`) |
| `-channel canary` | força um canal: `auto`, `stable`, `ptb` ou `canary` |
| `-exe "C:\..."` | aponta o executável na mão |
| `-no-update` | não consulta o GitHub atrás de versão nova |

## Como funciona por dentro

O launcher roda como o seu usuário. Ele acha o `Discord.exe` da versão mais nova em `%LOCALAPPDATA%`, lê o arquivo WireGuard e sobe uma segunda cópia de si mesmo com administrador, sem janela, que é quem mantém o túnel. Os dois conversam por arquivos na pasta de dados.

O processo do túnel cria um adaptador com o driver WireGuardNT e põe nele só as faixas do Discord, conferidas no registro do RIPE e no DNS: `66.22.192.0/18` e `195.62.89.0/24`, onde ficam os servidores de voz e vídeo, um bloco por região, e `162.159.128.0/20`, o trecho da Cloudflare com gateway, API e CDN, mais os equivalentes em IPv6. Na hora de abrir, ele ainda resolve os domínios do Discord e acrescenta qualquer endereço que tenha saído dessas faixas. Rota mais específica vence a rota padrão, então só esse tráfego entra no túnel, e o Windows escolhe isso sozinho para qualquer programa, inclusive o motor de mídia nativo do Discord.

O arquivo do Proton não traz endereço IPv6. As rotas IPv6 do Discord vão para o túnel mesmo assim: lá elas não têm saída, e a ideia é o Discord cair para o IPv4, que passa pelo túnel. Sem isso, numa rede com IPv6 parte do tráfego escaparia pelo IP brasileiro, que é exatamente a divergência que derrubou a 1.x.

Com o túnel de pé, o launcher pergunta ao gateway do Discord, em `gateway.discord.gg/cdn-cgi/trace`, com que IP e de que país você chega. É o IP do gateway que decide o que a conta pode fazer, e o pedido faz o mesmo caminho que o app vai fazer. O `location-metadata` do Discord, que responde a mesma pergunta, fica só de reserva: ele limita os pedidos por IP, e o servidor gratuito da VPN é dividido com muita gente. Se a resposta ainda for um país recusado, o túnel é desligado e a abertura para ali, em vez de entregar um Discord que parece liberado e não está. Só então o Discord abre, sem nenhum parâmetro especial.

O processo do túnel acompanha o `Discord.exe`. Quando o Discord fica fechado por mais de 30 segundos, prazo que cobre o reinício do próprio atualizador, ele desliga o adaptador, remove o driver se nenhum outro programa o estiver usando e sai.

## Compilar do código-fonte

Precisa do [Go](https://go.dev/dl/). No PowerShell, na pasta do projeto:

```
.\Compilar.ps1
```

É o mesmo script que a release usa: baixa o WireGuardNT do site oficial, confere o hash, embute o driver e a identificação no executável e compila. `go build` puro também compila, mas aí o executável procura o `wireguard.dll` na própria pasta.

| arquivo | conteúdo |
| --- | --- |
| `main.go` | opções, ordem das etapas, primeira abertura guiada, conferência da saída |
| `tunnel.go` | estado compartilhado entre o launcher e o processo do túnel |
| `tunnel_windows.go` | leitura do `.conf`, adaptador WireGuardNT, endereços, rotas e vigia do Discord |
| `elevate_windows.go` | pedido de administrador, abertura sem elevação e lista de processos |
| `routes.go` | faixas de IP do Discord e resolução dos domínios |
| `trace.go` | país e IP que o Discord enxerga |
| `wgfile.go` | busca do `.conf` nos Downloads e cópia guardada |
| `discord.go` | detecção da instalação, instância única, abertura |
| `ui.go` | etapas, spinner, perguntas, caixas de resumo e de erro |
| `update.go` | versão do build e consulta da última release no GitHub |
| `console*.go` | liga ANSI e UTF-8 no console, e detecta quando cair para texto puro |
| `*_test.go` | testes das partes que dão para checar sem administrador nem Discord instalado |

Os testes rodam sem rede e sem administrador:

```
go test ./...
```

Para conferir de verdade o que o Discord enxerga, sem o túnel ligado, há um teste que consulta o Discord e só roda quando pedido:

```
$env:DGLB_REDE = '1'; go test -run AoVivo -v ./...
```

## Créditos

A ideia de passar o Discord inteiro por um túnel WireGuard, em vez de só o gateway por um proxy, veio do **[GoLiveBypass](https://github.com/bezumiya/GoLiveBypass)**, que resolve o mesmo problema com interface gráfica e plugin para Vencord. Este projeto faz por fora, sem tocar na instalação do Discord. Por isso a licença aqui é a mesma dele, GPL-3.0.

O túnel usa o driver WireGuardNT e as bibliotecas `wireguard-windows`, da WireGuard LLC, cada um sob a sua licença. A do driver vai junto no `.zip`, em `LICENCA-WireGuardNT.txt`. WireGuard é marca registrada de Jason A. Donenfeld, e este projeto não tem relação com ele.
