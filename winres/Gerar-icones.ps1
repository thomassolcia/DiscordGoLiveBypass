# Gera os PNGs do icone a partir dos SVGs desta pasta. Rode de novo depois de mexer num
# SVG e faca commit dos PNGs: o Compilar.ps1 e a CI usam os PNGs prontos, sem precisar do
# Edge.
#
# Cada tamanho tem a sua versao. Do 256 ao 40 vai o desenho completo; em 32 e 24 o balao
# cresce e fica com um arco so de cada lado; em 20 e 16, que sao a bandeja e a barra de
# titulo, sai o fundo escuro para o balao ocupar o quadrado todo.
#
# O Edge desenha o SVG em 1024 x 1024 e o System.Drawing reduz para o tamanho final, o que
# deixa as bordas suaves mesmo em 16 px.

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing

$edge = @(
    "${env:ProgramFiles(x86)}\Microsoft\Edge\Application\msedge.exe",
    "$env:ProgramFiles\Microsoft\Edge\Application\msedge.exe"
) | Where-Object { Test-Path $_ } | Select-Object -First 1
if (-not $edge) { throw 'Edge nao encontrado, ele e usado para desenhar os SVGs.' }

$tamanhos = [ordered]@{
    256 = 'logo.svg'
    64  = 'logo.svg'
    48  = 'logo.svg'
    40  = 'logo.svg'
    32  = 'logo-pequeno.svg'
    24  = 'logo-pequeno.svg'
    20  = 'logo-solto.svg'
    16  = 'logo-solto.svg'
}

$temp = Join-Path ([IO.Path]::GetTempPath()) ("icones-" + [Guid]::NewGuid())
New-Item -ItemType Directory $temp | Out-Null
try {
    $desenhos = @{}
    foreach ($svg in ($tamanhos.Values | Sort-Object -Unique)) {
        $uri = ([Uri](Join-Path $PSScriptRoot $svg)).AbsoluteUri
        $html = Join-Path $temp "$svg.html"
        Set-Content -Encoding ascii $html "<!doctype html><style>html,body{margin:0;background:transparent}img{display:block;width:1024px;height:1024px}</style><img src=`"$uri`">"
        $png = Join-Path $temp "$svg.png"
        $parametros = @('--headless=new', '--disable-gpu', '--hide-scrollbars', '--force-device-scale-factor=1',
            '--default-background-color=00000000', '--window-size=1024,1024',
            "--user-data-dir=`"$temp\perfil`"", "--screenshot=`"$png`"", ([Uri]$html).AbsoluteUri)
        Start-Process -FilePath $edge -ArgumentList $parametros -Wait -WindowStyle Hidden
        if (-not (Test-Path $png)) { throw "o Edge nao desenhou $svg" }
        $desenhos[$svg] = [Drawing.Bitmap]::FromFile($png)
    }

    foreach ($par in $tamanhos.GetEnumerator()) {
        $tamanho = $par.Key
        $saida = New-Object Drawing.Bitmap $tamanho, $tamanho, ([Drawing.Imaging.PixelFormat]::Format32bppArgb)
        $g = [Drawing.Graphics]::FromImage($saida)
        $g.CompositingMode = [Drawing.Drawing2D.CompositingMode]::SourceCopy
        $g.InterpolationMode = [Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
        $g.PixelOffsetMode = [Drawing.Drawing2D.PixelOffsetMode]::HighQuality
        $atributos = New-Object Drawing.Imaging.ImageAttributes
        $atributos.SetWrapMode([Drawing.Drawing2D.WrapMode]::TileFlipXY)
        $g.DrawImage($desenhos[$par.Value], (New-Object Drawing.Rectangle 0, 0, $tamanho, $tamanho), 0, 0, 1024, 1024, [Drawing.GraphicsUnit]::Pixel, $atributos)
        $g.Dispose()
        $saida.Save((Join-Path $PSScriptRoot "icone-$tamanho.png"), [Drawing.Imaging.ImageFormat]::Png)
        $saida.Dispose()
        "icone-$tamanho.png"
    }
} finally {
    foreach ($imagem in $desenhos.Values) { $imagem.Dispose() }
    Remove-Item -Recurse -Force $temp -ErrorAction SilentlyContinue
}
