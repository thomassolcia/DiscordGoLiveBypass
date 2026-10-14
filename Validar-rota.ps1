# Confere se o tunel esta de pe e se o Discord realmente sai por ele, sem precisar de
# administrador. Serve para mandar junto de uma issue: nada aqui mostra chave da VPN.
#
# uso: .\Validar-rota.ps1   (com o Discord aberto pelo Abrir-Discord.bat)

$ErrorActionPreference = 'Continue'
$nome = 'DiscordGoLiveBypass'
$pasta = Join-Path $env:LOCALAPPDATA $nome

function Linha($rotulo, $valor, $cor = 'Gray') {
    Write-Host ("  {0,-22} {1}" -f $rotulo, $valor) -ForegroundColor $cor
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

# A mesma pergunta que o launcher faz: o pais que o proprio Discord diz enxergar.
function PaisDoDiscord {
    try {
        $resposta = Invoke-RestMethod -TimeoutSec 10 -Uri 'https://discord.com/api/v9/auth/location-metadata'
        return $resposta.country_code
    } catch {
        return "falhou: $($_.Exception.Message)"
    }
}

Write-Host ''
$estado = $null
$arquivo = Join-Path $pasta 'tunnel.json'
if (Test-Path -LiteralPath $arquivo) {
    $estado = Get-Content -Raw -LiteralPath $arquivo | ConvertFrom-Json
    $vivo = [bool](Get-Process -Id $estado.pid -ErrorAction SilentlyContinue)
    Linha 'estado do tunel' "$($estado.phase), pid $($estado.pid), $(if ($vivo) { 'rodando' } else { 'nao esta rodando' })"
    if ($estado.error) { Linha 'erro' $estado.error 'Red' }
    if ($estado.endpoint) { Linha 'servidor' $estado.endpoint }
    if ($estado.routes) { Linha 'faixas no tunel' $estado.routes }
} else {
    Linha 'estado do tunel' 'nenhum ainda, o launcher nunca subiu o tunel nesta conta' 'Yellow'
}

$adaptador = Get-NetAdapter -Name $nome -ErrorAction SilentlyContinue
Linha 'adaptador' $(if ($adaptador) { "$($adaptador.Status), $($adaptador.InterfaceDescription)" } else { 'nao existe agora' }) $(if ($adaptador) { 'Gray' } else { 'Yellow' })

Write-Host ''
Write-Host '  Por onde cada destino sai neste momento:'
$amostras = [ordered]@{
    'API e gateway'          = '162.159.128.233'
    'voz e video no Brasil'  = '66.22.200.10'
    'voz e video na Europa'  = '66.22.243.10'
    'controle: 1.1.1.1'      = '1.1.1.1'
}
foreach ($rotulo in $amostras.Keys) {
    $rota = Find-NetRoute -RemoteIPAddress $amostras[$rotulo] -ErrorAction SilentlyContinue | Where-Object { $_.InterfaceAlias } | Select-Object -First 1
    $interface = if ($rota) { $rota.InterfaceAlias } else { 'sem rota' }
    $noTunel = $interface -eq $nome
    $esperado = if ($rotulo -like 'controle*') { -not $noTunel } else { $noTunel }
    Linha $rotulo $interface $(if ($esperado) { 'Green' } else { 'Red' })
}

Write-Host ''
Write-Host '  Pais que cada servico enxerga:'
Linha 'Discord' (PaisDoDiscord)
Linha 'gateway do Discord' (Trace 'https://gateway.discord.gg/cdn-cgi/trace')
Linha 'resto do PC' (Trace 'https://www.cloudflare.com/cdn-cgi/trace')

$log = Join-Path $pasta 'tunnel.log'
if (Test-Path -LiteralPath $log) {
    Write-Host ''
    Write-Host '  Ultimas linhas do registro do tunel:'
    Get-Content -LiteralPath $log -Tail 12 | ForEach-Object { Write-Host "    $_" -ForegroundColor DarkGray }
}
Write-Host ''
