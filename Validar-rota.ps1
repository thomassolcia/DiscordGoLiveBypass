# Confere se o tunel esta de pe e se o Discord realmente sai por ele, sem precisar de
# administrador. Serve para mandar junto de uma issue: nada aqui mostra chave da VPN.
#
# uso: .\Validar-rota.ps1   (com o Discord aberto)

$ErrorActionPreference = 'Continue'
$nome = 'DiscordGoLiveBypass'

function Linha($rotulo, $valor, $cor = 'Gray') {
    Write-Host ("  {0,-24} {1}" -f $rotulo, $valor) -ForegroundColor $cor
}

function Trace($url) {
    try {
        $corpo = (Invoke-WebRequest -UseBasicParsing -TimeoutSec 10 -Uri $url).Content
        $loc = if ($corpo -match '(?m)^loc=(\w+)') { $matches[1] } else { '?' }
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
        Linha 'faixas no tunel' $st.routes
    } catch {
        Linha 'estado do tunel' "o servico nao respondeu: $($_.Exception.Message)" 'Yellow'
    }
}

$adaptador = Get-NetAdapter -Name $nome -ErrorAction SilentlyContinue
Linha 'adaptador' $(if ($adaptador) { "$($adaptador.Status), $($adaptador.InterfaceDescription)" } else { 'nao existe agora' }) $(if ($adaptador) { 'Gray' } else { 'Yellow' })

Write-Host ''
Write-Host '  Por onde cada destino sai neste momento:'
$amostras = [ordered]@{
    'API e gateway'          = '162.159.128.233'
    'voz e video, Sao Paulo' = '104.29.143.243'
    'voz e video, EUA'       = '104.29.158.178'
    'voz, bloco proprio'     = '66.22.200.10'
    'controle: 1.1.1.1'      = '1.1.1.1'
}
foreach ($rotulo in $amostras.Keys) {
    $rota = Find-NetRoute -RemoteIPAddress $amostras[$rotulo] -ErrorAction SilentlyContinue | Where-Object { $_.InterfaceAlias } | Select-Object -First 1
    $interface = if ($rota) { $rota.InterfaceAlias } else { 'sem rota' }
    $noTunel = $interface -eq $nome
    $esperado = if ($rotulo -like 'controle*') { -not $noTunel } else { $noTunel }
    Linha $rotulo $interface $(if ($esperado) { 'Green' } else { 'Red' })
}

# As amostras acima so provam que a lista de faixas cobre o que ja se conhecia. O log do
# proprio Discord diz a que servidor de midia ele conectou de verdade, e foi por ele que
# apareceu a mudanca da voz para a Cloudflare, quando todas as amostras estavam verdes.
$midia = Get-ChildItem -Path (Join-Path $env:APPDATA 'discord*\logs\renderer_js.log') -ErrorAction SilentlyContinue |
    Sort-Object LastWriteTime -Descending |
    ForEach-Object { Select-String -LiteralPath $_.FullName -Pattern 'Creating connection to (\d+\.\d+\.\d+\.\d+):\d+' | Select-Object -Last 1 } |
    Select-Object -First 1
if ($midia) {
    $ip = $midia.Matches[0].Groups[1].Value
    $quando = if ($midia.Line -match '^\[([\d-]+ [\d:]+)') { $matches[1] } else { '?' }
    $rota = Find-NetRoute -RemoteIPAddress $ip -ErrorAction SilentlyContinue | Where-Object { $_.InterfaceAlias } | Select-Object -First 1
    $interface = if ($rota) { $rota.InterfaceAlias } else { 'sem rota' }
    Write-Host ''
    Write-Host "  Ultimo servidor de voz e video do Discord ($quando):"
    Linha $ip $interface $(if ($interface -eq $nome) { 'Green' } else { 'Red' })
}

Write-Host ''
Write-Host '  Pais que cada servico enxerga:'
Linha 'gateway do Discord' (Trace 'https://gateway.discord.gg/cdn-cgi/trace')
Linha 'resto do PC' (Trace 'https://www.cloudflare.com/cdn-cgi/trace')

$log = Join-Path $env:APPDATA "$nome\janela.log"
if (Test-Path -LiteralPath $log) {
    Write-Host ''
    Write-Host '  Ultimas linhas do registro da janela:'
    Get-Content -LiteralPath $log -Tail 8 -Encoding UTF8 | ForEach-Object { Write-Host "    $_" -ForegroundColor DarkGray }
}
Write-Host ''
Write-Host '  O registro do servico fica em C:\ProgramData\DiscordGoLiveBypass\tunnel.log e so abre como administrador.' -ForegroundColor DarkGray
Write-Host ''
