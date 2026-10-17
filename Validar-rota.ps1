# Confere se o tunel esta de pe e se o Discord realmente sai por ele, sem precisar de
# administrador. Serve para mandar junto de uma issue: nada aqui mostra chave da VPN.
#
# uso: .\Validar-rota.ps1   (com o Discord aberto)

$ErrorActionPreference = 'Continue'
$nome = 'DiscordGoLiveBypass'
$st = $null
$modo = 'completo'

function Linha($rotulo, $valor, $cor = 'Gray') {
    Write-Host ("  {0,-24} {1}" -f $rotulo, $valor) -ForegroundColor $cor
}

# O IP de saida pela VPN e do servidor dela e ajuda a diagnosticar. O da conexao normal e o
# da pessoa, e este resultado costuma ir colado numa issue publica. Saida no Brasil quase
# sempre e a conexao normal, inclusive no gateway com o tunel desligado.
function Trace($url, [switch]$SemIp) {
    try {
        $corpo = (Invoke-WebRequest -UseBasicParsing -TimeoutSec 10 -Uri $url).Content
        $loc = if ($corpo -match '(?m)^loc=(\w+)') { $matches[1] } else { '?' }
        if ($SemIp -or $loc -eq 'BR') { return $loc }
        $ip = if ($corpo -match '(?m)^ip=(\S+)') { $matches[1] } else { '?' }
        return "$loc ($ip)"
    } catch {
        return "falhou: $($_.Exception.Message)"
    }
}

Write-Host ''
$servico = Get-Service -Name $nome -ErrorAction SilentlyContinue
if ($servico) {
    Linha 'servico' "$($servico.Status), inicio $($servico.StartType)" $(if ($servico.Status -eq 'Running') { 'Gray' } else { 'Yellow' })
} else {
    Linha 'servico' 'nao instalado' 'Yellow'
}

# O mesmo canal que a janela usa, so com o pedido de estado, que nao carrega nada da VPN.
if ($servico.Status -eq 'Running') {
    try {
        $canal = New-Object System.IO.Pipes.NamedPipeClientStream('.', $nome, [System.IO.Pipes.PipeDirection]::InOut)
        $canal.Connect(3000)
        $escrita = New-Object System.IO.StreamWriter($canal)
        $escrita.AutoFlush = $true
        $escrita.WriteLine('{"cmd":"status"}')
        $st = ((New-Object System.IO.StreamReader($canal)).ReadLine() | ConvertFrom-Json).status
        $canal.Dispose()

        Linha 'estado do tunel' "$($st.state), ligado: $($st.enabled), versao $($st.version)" $(if ($st.state -eq 'ligado') { 'Green' } else { 'Yellow' })
        if ($st.error) { Linha 'erro' $st.error 'Red' }
        if ($st.server) { Linha 'servidor' "$($st.server) ($($st.endpoint))" }
        Linha 'ultimo handshake' $(if ($st.handshake_s -lt 0) { 'nenhum ainda' } else { "ha $($st.handshake_s) s" })
        # Servico de antes da 2.2 nao informa o modo e sempre leva o Discord inteiro.
        if ($st.mode -eq 'transmissao') { $modo = 'transmissao' }
        Linha 'modo' $(if ($modo -eq 'transmissao') { 'so o Go Live' } else { 'o Discord inteiro' })
        Linha 'rotas no tunel' $st.routes
        if ($modo -eq 'transmissao') { Linha 'transmissoes no tunel' $st.streams }
    } catch {
        Linha 'estado do tunel' "o servico nao respondeu: $($_.Exception.Message)" 'Yellow'
    }
}

$adaptador = Get-NetAdapter -Name $nome -ErrorAction SilentlyContinue
Linha 'adaptador' $(if ($adaptador) { "$($adaptador.Status), $($adaptador.InterfaceDescription)" } else { 'nao existe agora' }) $(if ($adaptador) { 'Gray' } else { 'Yellow' })

function Interface($ip) {
    $rota = Find-NetRoute -RemoteIPAddress $ip -ErrorAction SilentlyContinue | Where-Object { $_.InterfaceAlias } | Select-Object -First 1
    if ($rota) { $rota.InterfaceAlias } else { 'sem rota' }
}

function PrimeiroIPv4($nomeDns) {
    try {
        [System.Net.Dns]::GetHostAddresses($nomeDns) | Where-Object { $_.AddressFamily -eq 'InterNetwork' } | Select-Object -First 1 | ForEach-Object { $_.ToString() }
    } catch { $null }
}

# Sem o gateway e a API no tunel o Go Live nao libera, e isso e vermelho nos dois modos. No
# modo transmissao, o resto do Discord deveria sair direto; se entrar no tunel, so fica mais
# lento, e por isso aparece em amarelo.
$transmissao = $modo -eq 'transmissao'
$amostras = @(
    @{ Rotulo = 'gateway';                Nome = 'gateway.discord.gg';    Tunel = $true }
    @{ Rotulo = 'API';                    Ip = '162.159.128.233';         Tunel = $true }
    @{ Rotulo = 'CDN e anexos';           Nome = 'cdn.discordapp.com';    Tunel = -not $transmissao }
    @{ Rotulo = 'teste de latencia';      Nome = 'latency.discord.media'; Tunel = -not $transmissao }
    @{ Rotulo = 'voz e video, Sao Paulo'; Ip = '104.29.143.243';          Tunel = -not $transmissao }
    @{ Rotulo = 'voz e video, EUA';       Ip = '104.29.158.178';          Tunel = -not $transmissao }
    @{ Rotulo = 'voz, bloco proprio';     Ip = '66.22.200.10';            Tunel = -not $transmissao }
    @{ Rotulo = 'controle: 1.1.1.1';      Ip = '1.1.1.1';                 Tunel = $false; Grave = $true }
)

Write-Host ''
Write-Host '  Por onde cada destino sai neste momento:'
foreach ($a in $amostras) {
    $ip = if ($a.Ip) { $a.Ip } else { PrimeiroIPv4 $a.Nome }
    if (-not $ip) {
        Linha $a.Rotulo "nao resolveu $($a.Nome)" 'Yellow'
        continue
    }
    $interface = Interface $ip
    $certo = ($interface -eq $nome) -eq $a.Tunel
    $grave = $a.Grave -or $a.Tunel
    Linha $a.Rotulo "$interface ($ip)" $(if ($certo) { 'Green' } elseif ($grave) { 'Red' } else { 'Yellow' })
}

# As amostras acima so provam que a lista de faixas cobre o que ja se conhecia. O log do
# proprio Discord diz a que servidor de midia ele conectou de verdade, e foi por ele que
# apareceu a mudanca da voz para a Cloudflare, quando todas as amostras estavam verdes.
$ultimoLog = Get-ChildItem -Path (Join-Path $env:APPDATA 'discord*\logs\renderer_js.log') -ErrorAction SilentlyContinue |
    Sort-Object LastWriteTime -Descending | Select-Object -First 1
if ($ultimoLog) {
    $conexoes = Select-String -LiteralPath $ultimoLog.FullName -Pattern '\[Connection\((default|stream)\)\] Creating connection to (\d+\.\d+\.\d+\.\d+):\d+'
    $mostrou = $false
    foreach ($tipo in 'default', 'stream') {
        $ultima = $conexoes | Where-Object { $_.Matches[0].Groups[1].Value -eq $tipo } | Select-Object -Last 1
        if (-not $ultima) { continue }
        if (-not $mostrou) {
            Write-Host ''
            Write-Host '  Ultimos servidores de midia no registro do Discord:'
            $mostrou = $true
        }
        $ip = $ultima.Matches[0].Groups[2].Value
        $quando = if ($ultima.Line -match '^\[([\d-]+ [\d:]+)') { $matches[1] } else { '?' }
        $interface = Interface $ip
        $noTunel = $interface -eq $nome
        if ($tipo -eq 'default') {
            $rotulo = 'voz'
            $cor = if ($noTunel -ne $transmissao) { 'Green' } elseif ($transmissao) { 'Yellow' } else { 'Red' }
        } else {
            # No modo transmissao, a rota so existe enquanto a transmissao esta aberta.
            $rotulo = 'transmissao'
            $aberta = (-not $transmissao) -or ($st.streams -gt 0)
            $cor = if (-not $aberta) { 'Gray' } elseif ($noTunel) { 'Green' } else { 'Red' }
            if (-not $aberta) { $interface = "$interface, sem transmissao aberta agora" }
        }
        Linha $rotulo "$ip, $interface ($quando)" $cor
    }
}

Write-Host ''
Write-Host '  Pais que cada servico enxerga:'
Linha 'gateway do Discord' (Trace 'https://gateway.discord.gg/cdn-cgi/trace')
Linha 'resto do PC' (Trace 'https://www.cloudflare.com/cdn-cgi/trace' -SemIp)

$log = Join-Path $env:APPDATA "$nome\janela.log"
if (Test-Path -LiteralPath $log) {
    Write-Host ''
    Write-Host '  Ultimas linhas do registro da janela:'
    Get-Content -LiteralPath $log -Tail 8 -Encoding UTF8 | ForEach-Object { Write-Host "    $_" -ForegroundColor DarkGray }
}
Write-Host ''
Write-Host '  O registro do servico fica em C:\ProgramData\DiscordGoLiveBypass\tunnel.log e so abre como administrador.' -ForegroundColor DarkGray
Write-Host ''
