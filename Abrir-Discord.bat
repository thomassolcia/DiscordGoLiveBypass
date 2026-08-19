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

echo Procurando um servidor fora do Brasil. Isso pode levar ate um minuto.
echo.

"%LAUNCHER%" -force %*

if errorlevel 1 (
    echo.
    echo ------------------------------------------------------------------
    echo Nao deu certo. A mensagem acima diz o motivo.
    echo O FAQ do README tem uma entrada para cada mensagem dessas.
    echo ------------------------------------------------------------------
    echo.
    pause
    exit /b 1
)

exit /b 0
