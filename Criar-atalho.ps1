# Cria um atalho na area de trabalho apontando para o launcher.
# Uso: clique com o botao direito neste arquivo e escolha "Executar com o PowerShell".

$ErrorActionPreference = 'Stop'

$pasta = Split-Path -Parent $MyInvocation.MyCommand.Path
$alvo = Join-Path $pasta 'DiscordGoLiveBypass.exe'

if (-not (Test-Path -LiteralPath $alvo)) {
    Write-Host ''
    Write-Host 'Nao encontrei o DiscordGoLiveBypass.exe nesta pasta.' -ForegroundColor Red
    Write-Host 'Baixe o programa na aba Releases e deixe os dois arquivos juntos.'
    Write-Host ''
    Read-Host 'Pressione Enter para fechar'
    exit 1
}

$icone = Get-ChildItem "$env:LOCALAPPDATA\Discord*\app-*\Discord*.exe" -ErrorAction SilentlyContinue |
    Sort-Object LastWriteTime -Descending |
    Select-Object -First 1 -ExpandProperty FullName

$destino = Join-Path ([Environment]::GetFolderPath('Desktop')) 'Discord sem bloqueio.lnk'

$shell = New-Object -ComObject WScript.Shell
$atalho = $shell.CreateShortcut($destino)
$atalho.TargetPath = $alvo
$atalho.Arguments = '-force'
$atalho.WorkingDirectory = $pasta
$atalho.Description = 'Abre o Discord por um servidor fora do Brasil'
if ($icone) { $atalho.IconLocation = $icone }
$atalho.Save()

Write-Host ''
Write-Host 'Pronto. O atalho "Discord sem bloqueio" esta na sua area de trabalho.' -ForegroundColor Green
Write-Host 'Use ele no lugar do atalho normal do Discord.'
Write-Host ''
Read-Host 'Pressione Enter para fechar'
