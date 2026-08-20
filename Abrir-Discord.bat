@echo off
rem Sem acentos de proposito: em muita maquina o console ainda usa codepage antiga.
title DiscordGoLiveBypass
cd /d "%~dp0"

rem Caminho absoluto: com NoDefaultCurrentDirectoryInExePath, o cmd nao acha o exe pelo nome.
set "LAUNCHER=%~dp0DiscordGoLiveBypass.exe"

if not exist "%LAUNCHER%" (
    echo.
    echo Nao encontrei o arquivo DiscordGoLiveBypass.exe nesta pasta.
    echo.
    echo Ele precisa estar do lado deste .bat. Baixe o .zip da aba Releases do
    echo repositorio e extraia tudo para uma pasta so, sem separar os arquivos.
    echo.
    pause
    exit /b 1
)

rem O proprio launcher desenha o cabecalho, as etapas e a caixa de erro: repetir isso
rem aqui so faria a tela dizer a mesma coisa duas vezes.
"%LAUNCHER%" -force %*

if errorlevel 1 (
    pause
    exit /b 1
)

rem Sem a espera a janela sumiria junto com o resumo do que foi feito.
echo   Esta janela fecha sozinha. O Discord ja esta abrindo.
timeout /t 8 > nul

exit /b 0
