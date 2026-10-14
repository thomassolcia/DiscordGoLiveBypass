# Compila o DiscordGoLiveBypass.exe com o driver do WireGuard embutido, exatamente como a
# release: o workflow chama este mesmo script, entao o binario de casa e o publicado saem
# do mesmo caminho.
#
# uso: .\Compilar.ps1               (build "dev", que nao avisa de versao nova)
#      .\Compilar.ps1 -Versao v2.0.0

[CmdletBinding()]
param(
    [string]$Versao = 'dev'
)

$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $PSScriptRoot

# Mesmo pacote e mesmo hash que o cliente oficial do WireGuard para Windows fixa no build
# dele. Baixar sem conferir o hash seria embutir no executavel qualquer coisa que viesse.
$driverUrl = 'https://download.wireguard.com/wireguard-nt/wireguard-nt-1.1.zip'
$driverHash = 'dceb30a9bc4be48cce0f74160fc88a585a2c2627366e8f846fc6658f9038dace'

$deps = Join-Path $PSScriptRoot '.deps'
$zip = Join-Path $deps 'wireguard-nt-1.1.zip'
New-Item -ItemType Directory -Force -Path $deps | Out-Null

if (-not (Test-Path -LiteralPath $zip) -or (Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash -ne $driverHash) {
    Write-Host 'Baixando o WireGuardNT'
    Invoke-WebRequest -UseBasicParsing -Uri $driverUrl -OutFile $zip
}
$hash = (Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash
if ($hash -ne $driverHash) {
    Remove-Item -LiteralPath $zip
    throw "O pacote do WireGuardNT nao bate com o hash esperado (veio $hash). Nada foi compilado."
}

$extraido = Join-Path $deps 'wireguard-nt'
Expand-Archive -Force -LiteralPath $zip -DestinationPath $extraido
Copy-Item -Force (Join-Path $extraido 'wireguard-nt\bin\amd64\wireguard.dll') (Join-Path $PSScriptRoot 'winres\wireguard.dll')
# A licenca do driver pede que ele siga acompanhado dela.
Copy-Item -Force (Join-Path $extraido 'wireguard-nt\LICENSE.txt') (Join-Path $PSScriptRoot 'LICENCA-WireGuardNT.txt')

# Recurso de versao so aceita numero: build fora de tag entra como 0.0.0.
$numero = $Versao -replace '^v', ''
if ($numero -notmatch '^\d+(\.\d+)*$') { $numero = '0.0.0' }

# Executavel sem nome, sem descricao e sem versao e o retrato do que antivirus heuristico
# marca. Os dados ficam em winres/winres.json, junto com o driver, e viram recurso do PE.
go run github.com/tc-hib/go-winres@v0.3.3 make --arch amd64 --file-version $numero --product-version $numero
if ($LASTEXITCODE -ne 0) { throw 'O go-winres falhou.' }

# Sem -s -w de proposito: binario pequeno e sem simbolos parece empacotado, que e
# justamente o padrao que a heuristica dos antivirus procura. Custa uns 4 MB.
# As tags desktop e production sao as que o Wails pede para gerar a janela sem o CLI dele,
# e o -H windowsgui faz o Windows abrir o programa sem um console atras.
go build -trimpath -tags "desktop,production,load_wgnt_from_rsrc" -ldflags "-H windowsgui -X main.version=$Versao" -o DiscordGoLiveBypass.exe .
if ($LASTEXITCODE -ne 0) { throw 'O go build falhou.' }

Write-Host "Pronto: DiscordGoLiveBypass.exe ($Versao), com o driver do WireGuard embutido."
