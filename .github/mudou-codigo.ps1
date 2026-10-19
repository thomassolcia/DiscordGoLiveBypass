# Diz ao ci.yml se a mudanca tem algo alem de documentacao. Quando so mudou texto, o job
# windows pula o vet, os testes e a compilacao e termina verde, em vez de gastar minutos de
# runner Windows num binario que nao mudou.
#
# uso: .github/mudou-codigo.ps1 -Base <commit>
#
# Compara o commit com o HEAD e grava codigo=true ou codigo=false no $env:GITHUB_OUTPUT
# (fora do Actions, so imprime). Na duvida responde true: sem base, ou com uma base que o
# clone nao tem, testar tudo e o unico jeito de nao deixar codigo passar sem conferencia.

param(
    [string]$Base = ''
)

# So o que nao entra no binario nem nos testes. Qualquer outro caminho conta como codigo,
# inclusive o resto de .github/: mudanca de workflow precisa ser testada rodando.
$documentacao = '^(README\.md|CONTRIBUTING\.md|docs/.+|\.github/notas-de-versao/.+)$'

function Responder([bool]$codigo) {
    $linha = 'codigo=' + $codigo.ToString().ToLower()
    if ($env:GITHUB_OUTPUT) {
        $linha | Out-File -Encoding ascii -Append $env:GITHUB_OUTPUT
    } else {
        $linha
    }
    exit 0
}

if (-not $Base) {
    Write-Host 'Sem commit para comparar: testando tudo.'
    Responder $true
}

git rev-parse --verify --quiet "$Base^{commit}" | Out-Null
if ($LASTEXITCODE -ne 0) {
    Write-Host "O clone nao tem $Base para comparar: testando tudo."
    Responder $true
}

# Sem --no-renames, um arquivo de codigo movido para docs/ apareceria so com o nome novo e
# a mudanca passaria por documentacao, mesmo tirando o arquivo do build.
$arquivos = @(git -c core.quotepath=off diff --no-renames --name-only $Base HEAD)
if ($LASTEXITCODE -ne 0) {
    Write-Host "O git nao conseguiu comparar com ${Base}: testando tudo."
    Responder $true
}

Write-Host "Arquivos alterados desde ${Base}:"
$arquivos | ForEach-Object { Write-Host "  $_" }

if (@($arquivos | Where-Object { $_ -notmatch $documentacao }).Count -gt 0) {
    Responder $true
}

Write-Host '::notice::So documentacao mudou: vet, testes e compilacao ficaram de fora.'
Responder $false
