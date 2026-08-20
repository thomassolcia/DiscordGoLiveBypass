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

rem Sem argumento nenhum, a origem e o duplo clique e vale perguntar. Quem chamou pela
rem linha de comando ja disse o que queria e nao pode ser interrompido por um menu.
set "CANAL="
if "%~1"=="" call :escolher

rem O proprio launcher desenha o cabecalho, as etapas e a caixa de erro: repetir isso
rem aqui so faria a tela dizer a mesma coisa duas vezes.
"%LAUNCHER%" -force %CANAL% %*

if errorlevel 1 (
    pause
    exit /b 1
)

rem Sem a espera a janela sumiria junto com o resumo do que foi feito.
echo   Esta janela fecha sozinha. O Discord ja esta abrindo.
timeout /t 8 > nul

exit /b 0

rem Menu de canal. So aparece com mais de um Discord instalado: perguntar para quem tem
rem um so seria um clique a mais para chegar na unica resposta possivel.
:escolher
setlocal enabledelayedexpansion
set /a TOTAL=0
call :achar Discord stable "Discord"
call :achar DiscordPTB ptb "Discord PTB"
call :achar DiscordCanary canary "Discord Canary"

rem Nenhum instalado tambem passa direto: a falta faz o launcher explicar o que houve,
rem bem melhor do que um menu vazio.
if !TOTAL! lss 2 exit /b 0

echo.
echo   Tem mais de um Discord instalado nesta maquina. Qual deles abrir?
echo.
for /l %%i in (1,1,!TOTAL!) do echo     [%%i] !NOME_%%i!
echo.

rem O choice come os espacos da frente da pergunta, entao ela fica na linha de cima e
rem aqui sobra so a entrada, que alinhada na margem ate parece mais um prompt.
choice /c !TECLAS! /n /m "Numero: "
set "ESCOLHIDO=!CHAVE_%errorlevel%!"
echo.

rem Escolha vazia e Ctrl+C, ou um Windows sem o choice. Nos dois casos e melhor deixar o
rem launcher detectar sozinho do que mandar um canal invalido para ele.
if "!ESCOLHIDO!"=="" exit /b 0

endlocal & set "CANAL=-channel %ESCOLHIDO%"
exit /b 0

rem Wildcard no meio do caminho nao funciona no if exist, entao quem procura a pasta
rem app-<versao> e o dir. Pasta vazia de desinstalacao antiga nao vira opcao.
:achar
dir /b /ad "%LOCALAPPDATA%\%~1\app-*" >nul 2>&1
if errorlevel 1 exit /b 0

set /a TOTAL+=1
set "CHAVE_!TOTAL!=%~2"
set "NOME_!TOTAL!=%~3"
set "TECLAS=!TECLAS!!TOTAL!"
exit /b 0
