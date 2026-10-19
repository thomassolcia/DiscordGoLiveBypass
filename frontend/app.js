'use strict';

// Cada função de window.go.main.App é um método do App em Go. Elas devolvem promessas, e
// um erro do lado do Go chega aqui como rejeição com o texto da mensagem.
const go = () => window.go.main.App;

const tela = document.getElementById('tela');
const rodape = document.getElementById('rodape');

const s = {
  app: null,
  prefs: { tema: 'sistema', abrir_discord: true },
  candidatos: [],
  escolhido: null,
  saida: null,
  conferidoEm: 0,
  ocupado: '',
  erro: '',
  avisoDiscord: false,
  versaoNova: '',
  confirmandoRemocao: false,
  troca: false,
  pedidoFechar: null,
  lembrarFechar: false,
  modoTrocado: false,
  iniciarComWindows: true,
  proton: novoProton(),
};

// Estado do login no Proton. A senha só existe aqui enquanto o login não termina.
function novoProton(aba = 'arquivo') {
  return { aba, etapa: 'login', email: '', senha: '', codigo: '', paises: [], pais: '', erro: '' };
}

const sobreposicao = document.getElementById('sobreposicao');

// Ícones desenhados à mão, no estilo de traço fino, para não depender de nada de fora.
const icones = {
  check: '<path d="M20 6 9 17l-5-5"/>',
  power: '<path d="M12 3v9"/><path d="M18.4 6.6a9 9 0 1 1-12.8 0"/>',
  alerta: '<path d="M12 9v4"/><path d="M12 17h.01"/><path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z"/>',
  gira: '<path d="M21 12a9 9 0 1 1-6.2-8.6"/>',
  play: '<path d="M8 5.5v13l10.5-6.5z"/>',
  arquivo: '<path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z"/><path d="M14 3v5h5"/>',
  soltar: '<path d="M12 15V4"/><path d="m7 9 5-5 5 5"/><path d="M5 15v3a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2v-3"/>',
  servidor: '<rect x="3" y="4" width="18" height="7" rx="2"/><rect x="3" y="13" width="18" height="7" rx="2"/><path d="M7 7.5h.01M7 16.5h.01"/>',
  escudo: '<path d="M12 3 5 6v6c0 4.4 3 7.6 7 9 4-1.4 7-4.6 7-9V6z"/>',
  raio: '<path d="M13 2 4 14h7l-1 8 9-12h-7z"/>',
  janela: '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 9h18"/>',
  globo: '<circle cx="12" cy="12" r="9"/><path d="M3 12h18"/><path d="M12 3a14 14 0 0 1 0 18 14 14 0 0 1 0-18z"/>',
  sistema: '<rect x="3" y="4" width="18" height="12" rx="2"/><path d="M8 20h8M12 16v4"/>',
  claro: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>',
  escuro: '<path d="M20.5 14.5A8.5 8.5 0 1 1 9.5 3.5a7 7 0 0 0 11 11z"/>',
  externo: '<path d="M14 4h6v6"/><path d="M10 14 20 4"/><path d="M19 13v6a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1h6"/>',
  recarregar: '<path d="M20 11a8 8 0 1 0-2.3 5.7"/><path d="M20 4v7h-7"/>',
  trocar: '<path d="m16 3 4 4-4 4"/><path d="M20 7H4"/><path d="m8 21-4-4 4-4"/><path d="M4 17h16"/>',
};

function icone(nome, preenchido = false) {
  return preenchido
    ? `<svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">${icones[nome]}</svg>`
    : `<svg class="traco" viewBox="0 0 24 24" aria-hidden="true">${icones[nome]}</svg>`;
}

function esc(texto) {
  return String(texto ?? '').replace(/[&<>"']/g, (c) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
  })[c]);
}

// O texto de erro vem do Go em minúscula, porque também aparece no meio de outras frases.
function frase(texto) {
  const t = String(texto ?? '').trim();
  return t ? t[0].toUpperCase() + t.slice(1) : '';
}

function bytes(n) {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(0)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`;
  return `${(n / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

// Tema ---------------------------------------------------------------------------------

const escuroDoSistema = window.matchMedia('(prefers-color-scheme: dark)');

function aplicarTema() {
  const escolhido = s.prefs.tema;
  const efetivo = escolhido === 'sistema' ? (escuroDoSistema.matches ? 'escuro' : 'claro') : escolhido;
  document.documentElement.dataset.theme = efetivo;
  for (const botao of document.querySelectorAll('[data-tema]')) {
    botao.setAttribute('role', 'radio');
    botao.setAttribute('aria-checked', String(botao.dataset.tema === escolhido));
    if (!botao.innerHTML) botao.innerHTML = icone(botao.dataset.tema);
  }
}

escuroDoSistema.addEventListener('change', aplicarTema);

async function salvarPreferencias(mudanca) {
  s.prefs = { ...s.prefs, ...mudanca };
  aplicarTema();
  desenhar();
  try {
    s.prefs = await go().SalvarPreferencias(s.prefs);
  } catch (e) {
    s.erro = String(e);
  }
  aplicarTema();
  desenhar();
}

// Estado --------------------------------------------------------------------------------

function configurado() {
  return Boolean(s.app?.service_running && s.app.status?.has_conf);
}

async function atualizar() {
  try {
    s.app = await go().Estado();
  } catch (e) {
    s.erro = String(e);
  }
  const st = s.app?.status;
  if (st?.state === 'ligado') {
    if (Date.now() - s.conferidoEm > 60000) conferirSaida();
  } else {
    s.saida = null;
    s.conferidoEm = 0;
  }
  if (!s.app?.discord_open?.length) s.avisoDiscord = false;
  desenhar();
}

async function conferirSaida() {
  s.conferidoEm = Date.now();
  try {
    s.saida = await go().ConferirSaida();
  } catch (e) {
    s.saida = { erro: String(e) };
  }
  desenhar();
}

async function acao(nome, fn) {
  if (s.ocupado) return;
  s.ocupado = nome;
  s.erro = '';
  desenhar();
  try {
    await fn();
  } catch (e) {
    s.erro = String(e);
  }
  s.ocupado = '';
  await atualizar();
}

// Com o túnel recém-ligado: um Discord fechado é aberto, se a pessoa quiser, e um Discord
// que já estava aberto ganha o aviso para reiniciar. Fechar sozinho poderia derrubar uma call.
async function depoisDeLigar(discordAberto) {
  if (discordAberto) {
    s.avisoDiscord = true;
  } else if (s.prefs.abrir_discord) {
    await go().AbrirDiscord().catch(() => {});
  }
}

// Com o túnel já configurado, um arquivo escolhido ou arrastado abre a troca de servidor.
function escolher(info) {
  if (!info || !info.path) return;
  if (configurado() && !s.troca) {
    s.troca = true;
    s.proton = novoProton();
  }
  s.escolhido = info;
  desenhar();
}

async function carregarCandidatos() {
  try {
    s.candidatos = await go().Candidatos();
    if (!s.escolhido && s.candidatos.length && !configurado()) s.escolhido = s.candidatos[0];
  } catch (e) {
    s.candidatos = [];
  }
  desenhar();
}

// Pedaços da tela -------------------------------------------------------------------------

function avisos() {
  const app = s.app;
  let html = '';
  // Durante a troca de servidor o erro aparece dentro da janela da troca, onde a pessoa está.
  if (s.erro && !s.troca) html += faixaErro(s.erro, 'erro');
  if (app?.error) html += faixaErro(app.error, 'erro-servico');
  if (app?.installed && !app.service_running) {
    html += `<div class="faixa atencao" data-chave="parado"><span>O serviço do túnel está instalado, mas parado.</span>
      <button data-acao="consertar" ${s.ocupado ? 'disabled' : ''}>Consertar</button></div>`;
  }
  if (app?.needs_update) {
    html += `<div class="faixa info" data-chave="atualizar"><span>Esta janela é da versão ${esc(app.version)} e o serviço instalado é da ${esc(app.status.version)}.</span>
      <button data-acao="atualizar" ${s.ocupado ? 'disabled' : ''}>${s.ocupado === 'atualizar' ? 'Atualizando…' : 'Atualizar'}</button></div>`;
  }
  if (s.versaoNova) {
    html += `<div class="faixa info" data-chave="versao"><span>Saiu a versão ${esc(s.versaoNova)}.</span>
      <button data-acao="baixar">${icone('externo')} Baixar</button></div>`;
  }
  return html;
}

function faixaErro(texto, chave) {
  return `<div class="faixa erro" data-chave="${chave}"><span>${esc(frase(texto))}</span></div>`;
}

function zonaSoltar(texto) {
  return `<div class="soltar" style="--wails-drop-target: drop">
      <span class="icone">${icone('soltar')}</span>
      <span class="textos">${texto}</span>
      <button data-acao="escolher">${icone('arquivo')} Escolher</button>
    </div>`;
}

function listaArquivos() {
  const itens = [...s.candidatos];
  if (s.escolhido && !itens.some((c) => c.path === s.escolhido.path)) itens.unshift(s.escolhido);
  if (!itens.length) return '';

  let html = '<div class="opcoes">';
  for (const c of itens) {
    if (c.error) continue;
    const origem = c.origin === '2.0' ? 'guardado pela versão 2.0'
      : c.origin === 'downloads' ? `nos Downloads, ${esc(c.modified)}` : esc(c.modified);
    const marcado = s.escolhido?.path === c.path ? 'escolhida' : '';
    html += `<button class="opcao ${marcado}" data-arquivo="${esc(c.path)}">
        <span class="marcador"></span>
        <span class="textos"><b>${esc(c.server || c.name)}</b><small>${esc(c.name)} · ${origem}</small></span>
      </button>`;
  }
  return html + '</div>';
}

function abasArquivo() {
  const aba = s.proton.aba;
  return `<div class="abas" role="tablist">
      <button role="tab" data-aba="arquivo" aria-selected="${aba === 'arquivo'}">Tenho o arquivo .conf</button>
      <button role="tab" data-aba="proton" aria-selected="${aba === 'proton'}">Entrar com o Proton</button>
    </div>`;
}

function formProton() {
  const p = s.proton;
  const ocupado = s.ocupado === 'proton';
  const erro = p.erro ? faixaErro(p.erro, 'erro-proton') : '';

  if (p.etapa === 'verificacao') {
    return `<div class="esperando" data-chave="proton-verificacao">${icone('gira')}<span>O Proton pediu uma verificação de segurança. Resolva na janela que abriu: ela fecha sozinha quando terminar.</span></div>`;
  }

  if (p.etapa === 'codigo') {
    return `<div class="formulario" data-chave="proton-codigo">${erro}
        <label class="campo"><span>Código de verificação em duas etapas</span>
          <input class="codigo" data-campo="codigo" inputmode="numeric" maxlength="7" autocomplete="one-time-code" value="${esc(p.codigo)}" placeholder="000000"></label>
        <div class="linha-acoes">
          <button class="link" data-acao="proton-cancelar">Voltar</button>
          <button class="principal" data-acao="proton-entrar" ${ocupado ? 'disabled' : ''}>${ocupado ? 'Conferindo…' : 'Confirmar'}</button>
        </div>
      </div>`;
  }

  if (p.etapa === 'paises') {
    const automatico = p.paises.length ? `Automático: ${p.paises[0].melhor}, o melhor agora` : 'Automático';
    const opcoes = p.paises.map((pais) => `<option value="${esc(pais.code)}" ${p.pais === pais.code ? 'selected' : ''}>
        ${esc(pais.label)} · ${pais.quantos} ${pais.quantos === 1 ? 'servidor' : 'servidores'}</option>`).join('');
    return `<div class="formulario" data-chave="proton-paises">${erro}
        <label class="campo"><span>Servidor gratuito</span>
          <select data-campo="pais"><option value="" ${p.pais === '' ? 'selected' : ''}>${esc(automatico)}</option>${opcoes}</select></label>
        <div class="linha-acoes">
          <button class="link" data-acao="proton-cancelar">Voltar</button>
          <button class="principal" data-acao="proton-gerar" ${ocupado ? 'disabled' : ''}>${ocupado ? 'Gerando…' : 'Gerar o arquivo'}</button>
        </div>
      </div>`;
  }

  return `<div class="formulario" data-chave="proton-login">${erro}
      <label class="campo"><span>E-mail da conta do Proton</span>
        <input type="email" data-campo="email" autocomplete="username" value="${esc(p.email)}"></label>
      <label class="campo"><span>Senha</span>
        <input type="password" data-campo="senha" autocomplete="current-password" value="${esc(p.senha)}"></label>
      <button class="principal largo" data-acao="proton-entrar" ${ocupado ? 'disabled' : ''}>${ocupado ? 'Entrando…' : 'Entrar'}</button>
      <p class="pequeno">A senha não fica gravada. Não tem conta? <button class="link pequeno" data-acao="proton-criar">Crie uma grátis</button></p>
    </div>`;
}

// A escolha do arquivo é a mesma na primeira tela e na troca de servidor. A área tem altura
// fixa para nada pular quando a pessoa troca de aba ou avança no login.
function escolhaArquivo(textoSoltar) {
  const aba = s.proton.aba;
  return `${abasArquivo()}
    <div class="escolha" data-chave="escolha-${aba}">
      ${aba === 'proton' ? formProton() : `
        ${listaArquivos()}
        ${s.escolhido?.error ? faixaErro(s.escolhido.error, 'erro-arquivo') : ''}
        ${zonaSoltar(textoSoltar)}`}
    </div>`;
}

function marcarAbrirDiscord() {
  const nome = s.app?.discord || 'Discord';
  return `<label class="marcar">
      <input type="checkbox" data-pref="abrir_discord" ${s.prefs.abrir_discord ? 'checked' : ''}>
      <span>Abrir o ${esc(nome)} quando o túnel ligar</span>
    </label>`;
}

function telaInicial() {
  const jaInstalado = s.app?.service_running;
  // Na aba do Proton, o arquivo marcado na outra aba não é o que a pessoa está olhando.
  const pronto = s.proton.aba === 'arquivo' && s.escolhido && !s.escolhido.error;
  const ativando = s.ocupado === 'ativar';

  return `${avisos()}
    <div class="inicio" data-chave="inicio">
      <section class="apresentacao">
        <span class="rotulo">Go Live no Brasil</span>
        <h1>Seu Go Live de volta no Discord</h1>
        <p>Um túnel WireGuard leva só o Go Live do Discord para fora do Brasil. A voz da chamada, o jogo, o navegador e o resto do computador continuam na sua conexão.</p>
        <ul class="vantagens">
          <li><span class="icone">${icone('escudo')}</span><div><b>Só a transmissão passa pela VPN</b><span>Voz sem atraso e nada de ping alto no jogo.</span></div></li>
          <li><span class="icone">${icone('raio')}</span><div><b>Liga sozinho com o Windows</b><span>Depois de ativar, é só abrir o Discord pelo atalho de sempre.</span></div></li>
          <li><span class="icone">${icone('janela')}</span><div><b>Sem janela preta</b><span>A permissão de administrador é pedida uma vez só.</span></div></li>
        </ul>
        <div class="cartao dica">
          <h2>${icone('globo')} Ainda não tem o arquivo?</h2>
          <ol>
            <li>Crie uma conta gratuita no <b>Proton VPN</b>.</li>
            <li>Em <b>Downloads</b>, vá até <b>WireGuard configuration</b>.</li>
            <li>Escolha <b>Windows</b> e um servidor gratuito fora do Brasil.</li>
            <li>Clique em <b>Create</b> e baixe. O arquivo aparece aqui sozinho.</li>
          </ol>
          <button class="link" data-acao="proton">${icone('externo')} Abrir a página do Proton</button>
        </div>
      </section>

      <div class="coluna">
        <section class="cartao passos">
          <h2><span class="passo">1</span> Arquivo da VPN</h2>
          ${escolhaArquivo('Arraste o arquivo <b>.conf</b> para cá')}
          <hr>
          <h2><span class="passo">2</span> Ativar</h2>
          ${marcarAbrirDiscord()}
          <label class="marcar">
            <input type="checkbox" data-iniciar ${s.iniciarComWindows ? 'checked' : ''}>
            <span>Iniciar com o Windows, com o ícone na bandeja</span>
          </label>
          <button class="principal grande largo" data-acao="ativar" ${pronto && !s.ocupado ? '' : 'disabled'}
            title="${jaInstalado ? 'O serviço do túnel já está instalado' : 'O Windows pede permissão de administrador uma única vez'}">
            ${ativando ? 'Ativando…' : 'Ativar'}</button>
        </section>
      </div>
    </div>`;
}

function cartaoEstado(st) {
  const app = s.app;
  let classe = 'ruim';
  let simbolo = icone('alerta');
  let titulo = 'Algo deu errado';
  let detalhe = esc(frase(st.error));
  // Um Discord aberto antes do túnel precisa reiniciar. O aviso entra no lugar da frase do
  // estado e o botão principal vira o de reiniciar, sem o cartão crescer.
  const abertos = app.discord_open || [];
  const reiniciar = st.state === 'ligado' && s.avisoDiscord && abertos.length > 0 && !s.saida?.blocked;
  const nomeAberto = abertos.length === 1 ? abertos[0] : 'Discord';

  switch (st.state) {
    case 'ligado':
      if (s.saida?.blocked) {
        titulo = 'O Discord ainda vê o Brasil';
        detalhe = 'O servidor escolhido não tirou você do Brasil. Troque o arquivo da VPN por outro servidor.';
        classe = 'alerta';
      } else {
        titulo = 'Go Live liberado';
        detalhe = reiniciar
          ? `<span class="atencao-texto">O ${esc(nomeAberto)} já estava aberto. Reinicie para ele entrar no túnel.</span>`
          : s.modoTrocado
            ? '<span class="atencao-texto">Modo trocado. Saia e entre de novo na chamada.</span>'
            : st.mode === 'completo'
              ? 'O Discord inteiro está passando pelo túnel.'
              : st.streams > 0
                ? 'A transmissão está passando pelo túnel.'
                : 'Só o Go Live passa pelo túnel.';
        classe = 'ok';
        simbolo = icone('check');
      }
      break;
    case 'conectando':
      titulo = 'Conectando…';
      detalhe = 'Esperando o servidor da VPN responder.';
      classe = 'espera';
      simbolo = icone('gira');
      break;
    case 'sem_resposta':
      titulo = 'Servidor sem resposta';
      classe = 'alerta';
      break;
    case 'desligado':
      titulo = 'Túnel desligado';
      detalhe = 'O Discord sai pela sua conexão normal, sem Go Live.';
      classe = 'desligado';
      simbolo = icone('power');
      break;
  }

  let saida = '';
  if (st.state === 'ligado') {
    if (!s.saida) {
      saida = `<div class="saida"><span class="pais">··</span><span class="textos"><b>Conferindo o que o Discord vê…</b></span></div>`;
    } else if (s.saida.erro) {
      saida = `<div class="saida"><span class="pais">?</span><span class="textos"><b>Não deu para conferir a saída</b><small>${esc(frase(s.saida.erro))}</small></span></div>`;
    } else {
      saida = `<div class="saida"><span class="pais">${esc(s.saida.country)}</span>
        <span class="textos"><b>O Discord vê você em ${esc(s.saida.label.replace(/ \(\w+\)$/, ''))}</b>
        <small>IP ${esc(s.saida.ip)} · ${s.saida.ms} ms</small></span></div>`;
    }
  }

  let discord = app.discord
    ? `<button class="principal grande" data-acao="abrir-discord" ${s.ocupado ? 'disabled' : ''}>${icone('play', true)} Abrir o ${esc(app.discord)}</button>`
    : '';
  if (reiniciar) {
    discord = `<button class="principal grande" data-acao="reiniciar" ${s.ocupado ? 'disabled' : ''}>${icone('recarregar')}
      ${s.ocupado === 'reiniciar' ? 'Reiniciando…' : `Reiniciar o ${esc(nomeAberto)}`}</button>`;
  }
  const alternar = st.enabled
    ? `<button class="grande" data-acao="desligar" ${s.ocupado ? 'disabled' : ''}>${icone('power')} ${s.ocupado === 'desligar' ? 'Desligando…' : 'Desligar'}</button>`
    : `<button class="principal grande" data-acao="ligar" ${s.ocupado ? 'disabled' : ''}>${icone('power')} ${s.ocupado === 'ligar' ? 'Ligando…' : 'Ligar o túnel'}</button>`;

  return `<section class="cartao estado ${classe}">
      <div class="topo">
        <span class="selo">${simbolo}</span>
        <div><div class="titulo">${esc(titulo)}</div><div class="detalhe">${detalhe}</div></div>
      </div>
      ${saida}
      <div class="acoes">${st.enabled ? discord + alternar : alternar}</div>
    </section>`;
}

// O que passa pela VPN. Serviço de antes da 2.2 não diz o modo e não sabe trocar, então a
// escolha só aparece depois da atualização.
function escolhaModo(st) {
  if (!st.mode) return '';
  const explicacao = st.mode === 'completo'
    ? 'A câmera funciona, mas a voz passa pela VPN, com mais atraso.'
    : 'Voz e mensagens seguem direto. A câmera não abre para quem assiste.';
  return `<label class="linha-pref">
      <span>Pela VPN</span>
      <select data-chave="modo" data-modo ${s.ocupado ? 'disabled' : ''}>
        <option value="transmissao" ${st.mode === 'transmissao' ? 'selected' : ''}>Só o Go Live</option>
        <option value="completo" ${st.mode === 'completo' ? 'selected' : ''}>O Discord inteiro</option>
      </select>
    </label>
    <p class="pequeno modo" data-chave="modo-texto">${explicacao}</p>`;
}

// Duas linhas de dois cartões. A grade iguala a altura dos cartões de cada linha, e o que
// cresce num deles cresce a linha inteira junto.
function painel() {
  const st = s.app.status;
  const handshake = st.handshake_s < 0 ? 'nenhum ainda' : `há ${st.handshake_s} s`;

  return `${avisos()}
    <div class="painel" data-chave="painel">
      ${cartaoEstado(st)}
      <section class="cartao">
        <h2>Servidor da VPN</h2>
        <div class="servidor">
          <span class="icone">${icone('servidor')}</span>
          <div><b>${esc(st.server || '-')}</b><small>${esc(st.endpoint || '')}</small></div>
        </div>
        <p class="pequeno">Dá para trocar por outro arquivo <b>.conf</b> ou gerar um novo pela sua conta do Proton. Arrastar o arquivo para a janela também funciona.</p>
        <div class="acoes fim">
          <button class="grande" data-acao="abrir-troca" ${s.ocupado ? 'disabled' : ''}>${icone('trocar')} Trocar de servidor</button>
        </div>
      </section>
      <section class="cartao">
        <h2>Preferências</h2>
        ${escolhaModo(st)}
        ${marcarAbrirDiscord()}
        <label class="linha-pref">
          <span>Ao clicar no X</span>
          <select data-pref="ao_fechar">
            <option value="perguntar" ${s.prefs.ao_fechar === 'perguntar' ? 'selected' : ''}>Perguntar</option>
            <option value="bandeja" ${s.prefs.ao_fechar === 'bandeja' ? 'selected' : ''}>Ir para a bandeja</option>
            <option value="parar" ${s.prefs.ao_fechar === 'parar' ? 'selected' : ''}>Desligar o túnel e fechar</option>
          </select>
        </label>
        <p class="pequeno fim">Minimizar leva a janela para a bandeja. O túnel não depende dela.</p>
      </section>
      <section class="cartao">
        <h2>Detalhes técnicos</h2>
        <div class="par"><span>Último handshake</span><b>${esc(handshake)}</b></div>
        <div class="par"><span>Recebido / enviado</span><b>${bytes(st.rx)} / ${bytes(st.tx)}</b></div>
        <div class="par"><span>Rotas do Discord no túnel</span><b>${st.routes}</b></div>
        ${st.mode === 'transmissao' ? `<div class="par" data-chave="transmissoes"><span>Transmissões no túnel</span><b>${st.streams || 0}</b></div>` : ''}
        <div class="par"><span>Versão do serviço</span><b>${esc(st.version)}</b></div>
      </section>
    </div>`;
}

// A troca de servidor abre por cima do painel, com as mesmas abas da primeira tela, em vez
// de esticar o cartão do servidor e empurrar o resto da janela.
function modalTroca() {
  if (!s.troca) return '';
  const pronto = s.escolhido && !s.escolhido.error;
  const nome = pronto ? esc(s.escolhido.server || s.escolhido.name) : '';
  const confirmar = s.proton.aba === 'arquivo'
    ? `<button class="principal" data-acao="trocar" ${pronto && !s.ocupado ? '' : 'disabled'}>
        ${s.ocupado === 'trocar' ? 'Trocando…' : pronto ? `Trocar para ${nome}` : 'Escolha um arquivo'}</button>`
    : '';
  return `<div class="fundo-modal" data-acao="fechar-troca" data-chave="modal-troca">
      <section class="modal largo" role="dialog" aria-modal="true" aria-labelledby="titulo-troca">
        <h2 id="titulo-troca">Trocar o servidor da VPN</h2>
        ${s.erro ? faixaErro(s.erro, 'erro-troca') : ''}
        ${escolhaArquivo('Arraste outro arquivo <b>.conf</b> para cá')}
        <div class="linha-acoes rodape-modal">
          <button class="link" data-acao="fechar-troca" ${s.ocupado ? 'disabled' : ''}>Cancelar</button>
          ${confirmar}
        </div>
      </section>
    </div>`;
}

function desenharRodape() {
  if (!s.app) return '';
  let direita = '';
  if (s.app.installed) {
    direita = s.confirmandoRemocao
      ? `<span class="confirmar">Remover o serviço e o túnel deste computador?
          <button class="perigo" data-acao="remover">Remover</button>
          <button class="link" data-acao="cancelar-remocao">Cancelar</button></span>`
      : `<button class="link" data-acao="pedir-remocao">Desinstalar</button>`;
  }
  return `<span class="lado"><button class="link" data-acao="ajuda">${icone('externo')} Ajuda e perguntas frequentes</button></span>
    <span class="lado">${direita}</span>`;
}

// A pergunta do X. Deixar na bandeja vem primeiro porque é o que mantém o Go Live
// funcionando, e é o que a maioria das pessoas quer ao fechar a janela.
function modalFechar() {
  const pedido = s.pedidoFechar;
  if (!pedido) return '';
  const ocupado = s.ocupado === 'fechar' ? 'disabled' : '';
  return `<div class="fundo-modal" data-acao="fechar-cancelar" data-chave="modal-fechar">
      <section class="modal" role="dialog" aria-modal="true" aria-labelledby="titulo-fechar">
        <h2 id="titulo-fechar">Fechar o DiscordGoLiveBypass?</h2>
        <p>${pedido.bandeja
          ? 'Na bandeja, perto do relógio, o túnel continua ligado e o Go Live segue funcionando. Desligar o túnel faz o Discord voltar para a sua conexão normal, sem Go Live.'
          : 'Desligar o túnel faz o Discord voltar para a sua conexão normal, sem Go Live.'}</p>
        <div class="acoes">
          ${pedido.bandeja ? `<button class="principal grande" data-acao="fechar-bandeja" ${ocupado}>Deixar na bandeja</button>` : ''}
          <button class="grande perigo-cheio" data-acao="fechar-parar" ${ocupado}>${s.ocupado === 'fechar' ? 'Desligando…' : 'Desligar o túnel e fechar'}</button>
        </div>
        <div class="rodape-modal">
          <label class="marcar"><input type="checkbox" data-lembrar ${s.lembrarFechar ? 'checked' : ''}><span>Não perguntar de novo</span></label>
          <button class="link" data-acao="fechar-cancelar">Cancelar</button>
        </div>
      </section>
    </div>`;
}

// A tela é montada de novo a cada consulta ao serviço, de 3 em 3 segundos, mas só o que
// mudou de fato vai para a página. Trocar tudo de uma vez recriava o campo em que a pessoa
// estava digitando, e o cursor voltava para o começo; também fechava listas abertas e
// piscava o botão sob o mouse.
function desenhar() {
  if (!s.app) return;
  document.getElementById('versao').textContent = s.app.version;
  aplicar(tela, configurado() ? painel() : telaInicial());
  aplicar(rodape, desenharRodape());
  aplicar(sobreposicao, modalTroca() + modalFechar());
}

function aplicar(alvo, html) {
  const modelo = document.createElement('template');
  modelo.innerHTML = html;
  juntarFilhos(alvo, modelo.content);
}

// Dois elementos são o mesmo quando têm a mesma tag, a mesma chave e a mesma primeira
// classe. A chave separa o que pode aparecer e sumir no meio da lista, como os avisos, e
// também os campos: o e-mail nunca é reaproveitado como campo do código.
function chave(no) {
  if (no.nodeType !== Node.ELEMENT_NODE) return '';
  const d = no.dataset || {};
  return no.id || d.chave || d.campo || d.pref || d.arquivo || '';
}

function mesmo(a, b) {
  if (a.nodeType !== b.nodeType || a.nodeName !== b.nodeName) return false;
  if (a.nodeType !== Node.ELEMENT_NODE) return true;
  return chave(a) === chave(b) && (a.classList[0] || '') === (b.classList[0] || '')
    && (a.nodeName !== 'INPUT' || a.type === b.type);
}

function juntarFilhos(atual, novo) {
  let cursor = atual.firstChild;
  for (const filho of [...novo.childNodes]) {
    let par = cursor;
    if (chave(filho)) {
      while (par && !mesmo(par, filho)) par = par.nextSibling;
    } else if (par && !mesmo(par, filho)) {
      par = null;
    }
    if (!par) {
      atual.insertBefore(filho, cursor);
      continue;
    }
    if (par !== cursor) atual.insertBefore(par, cursor);
    juntar(par, filho);
    cursor = par.nextSibling;
  }
  while (cursor) {
    const proximo = cursor.nextSibling;
    cursor.remove();
    cursor = proximo;
  }
}

function juntar(atual, novo) {
  if (atual.nodeType !== Node.ELEMENT_NODE) {
    if (atual.nodeValue !== novo.nodeValue) atual.nodeValue = novo.nodeValue;
    return;
  }
  for (const { name, value } of [...novo.attributes]) {
    if (atual.getAttribute(name) !== value) atual.setAttribute(name, value);
  }
  for (const { name } of [...atual.attributes]) {
    if (!novo.hasAttribute(name)) atual.removeAttribute(name);
  }
  // Lido antes de juntar os filhos, que levam as opções novas de um select para a página.
  const valor = novo.value;
  const marcado = novo.checked;
  juntarFilhos(atual, novo);

  // Depois que a pessoa mexe num campo, o atributo value não muda mais o que aparece nele.
  // O estado manda nos campos parados; o que está com o foco fica como a pessoa deixou.
  if (atual === document.activeElement) return;
  if (atual.nodeName === 'INPUT' && atual.type === 'checkbox') atual.checked = marcado;
  else if (atual.nodeName === 'INPUT' && atual.value !== valor) atual.value = valor;
  else if (atual.nodeName === 'SELECT' && atual.value !== valor) atual.value = valor;
}

// Como acao(), mas o erro aparece dentro do formulário do Proton, onde a pessoa está olhando.
async function acaoProton(fn) {
  if (s.ocupado) return;
  s.ocupado = 'proton';
  s.proton.erro = '';
  desenhar();
  try {
    await fn();
  } catch (e) {
    s.proton.erro = String(e);
    if (s.proton.etapa === 'verificacao') s.proton.etapa = 'login';
  }
  s.ocupado = '';
  desenhar();
}

// Ações -------------------------------------------------------------------------------

const acoes = {
  escolher: async () => escolher(await go().EscolherArquivo()),
  proton: () => go().AbrirProton(),
  ajuda: () => go().AbrirAjuda(),
  baixar: () => go().AbrirRelease(),
  ativar: () => acao('ativar', async () => {
    const discordAberto = (s.app?.discord_open?.length || 0) > 0;
    await go().Ativar(s.escolhido.path, s.iniciarComWindows);
    s.escolhido = null;
    await depoisDeLigar(discordAberto);
  }),
  ligar: () => acao('ligar', async () => {
    const discordAberto = (s.app?.discord_open?.length || 0) > 0;
    await go().Ligar();
    await depoisDeLigar(discordAberto);
  }),
  desligar: () => acao('desligar', () => go().Desligar()),
  'abrir-discord': () => acao('abrir-discord', () => go().AbrirDiscord()),
  trocar: () => acao('trocar', async () => {
    await go().TrocarArquivo(s.escolhido.path);
    s.troca = false;
    s.escolhido = null;
    s.proton = novoProton();
    s.saida = null;
    s.conferidoEm = 0;
  }),
  'abrir-troca': () => {
    s.troca = true;
    s.escolhido = null;
    s.erro = '';
    s.proton = novoProton();
    desenhar();
    carregarCandidatos();
  },
  // Fechar no meio do login encerra a sessão que o Proton já tinha aberto.
  'fechar-troca': () => {
    if (s.ocupado) return;
    if (s.proton.etapa === 'paises') go().ProtonCancelar();
    s.troca = false;
    s.escolhido = null;
    s.erro = '';
    s.proton = novoProton();
    desenhar();
  },
  reiniciar: () => acao('reiniciar', async () => {
    await go().ReiniciarDiscord();
    s.avisoDiscord = false;
  }),
  atualizar: () => acao('atualizar', () => go().Atualizar()),
  consertar: () => acao('consertar', () => go().Atualizar()),
  'pedir-remocao': () => { s.confirmandoRemocao = true; desenhar(); },
  'cancelar-remocao': () => { s.confirmandoRemocao = false; desenhar(); },
  remover: () => acao('remover', () => go().Desinstalar()),
  'fechar-bandeja': async () => {
    const lembrar = s.lembrarFechar;
    s.pedidoFechar = null;
    s.lembrarFechar = false;
    desenhar();
    await go().FecharParaBandeja(lembrar);
    if (lembrar) s.prefs = await go().Preferencias();
  },
  'fechar-parar': () => acao('fechar', async () => {
    await go().PararEFechar(s.lembrarFechar);
    s.pedidoFechar = null;
  }),
  'fechar-cancelar': () => { s.pedidoFechar = null; s.lembrarFechar = false; desenhar(); },
  'proton-criar': () => go().AbrirCadastroProton(),
  'proton-entrar': () => acaoProton(async () => {
    const p = s.proton;
    const resposta = await go().ProtonEntrar({ email: p.email, senha: p.senha, codigo: p.codigo });
    if (resposta.etapa === 'codigo') {
      if (p.etapa === 'codigo' && p.codigo) p.erro = 'código de verificação incorreto ou vencido, confira no aplicativo e tente de novo';
      p.etapa = 'codigo';
      return;
    }
    // Daqui em diante a senha não serve para mais nada.
    p.senha = '';
    p.codigo = '';
    p.paises = resposta.paises;
    p.pais = '';
    p.etapa = 'paises';
  }),
  'proton-gerar': () => acaoProton(async () => {
    const info = await go().ProtonGerar(s.proton.pais);
    s.candidatos = [info, ...s.candidatos.filter((c) => c.origin !== 'proton')];
    s.escolhido = info;
    s.proton = novoProton('arquivo');
  }),
  'proton-cancelar': () => {
    go().ProtonCancelar();
    s.proton = { ...novoProton('proton'), email: s.proton.email };
    desenhar();
  },
};

document.addEventListener('click', (evento) => {
  const tema = evento.target.closest('[data-tema]');
  if (tema) {
    salvarPreferencias({ tema: tema.dataset.tema });
    return;
  }
  const aba = evento.target.closest('[data-aba]');
  if (aba) {
    if (s.ocupado !== 'proton') s.proton.aba = aba.dataset.aba;
    desenhar();
    return;
  }
  const alvo = evento.target.closest('[data-acao], [data-arquivo]');
  if (!alvo || alvo.disabled) return;
  // O fundo escurecido fecha a pergunta, mas um clique dentro do cartão não pode subir até ele.
  if (alvo.classList.contains('fundo-modal') && evento.target !== alvo) return;
  if (alvo.dataset.arquivo) {
    escolher([...s.candidatos, s.escolhido].find((c) => c && c.path === alvo.dataset.arquivo));
    return;
  }
  const fn = acoes[alvo.dataset.acao];
  if (fn) fn();
});

document.addEventListener('change', (evento) => {
  if (evento.target.matches('[data-lembrar]')) {
    s.lembrarFechar = evento.target.checked;
    return;
  }
  if (evento.target.matches('[data-iniciar]')) {
    s.iniciarComWindows = evento.target.checked;
    return;
  }
  if (evento.target.matches('[data-modo]')) {
    const modo = evento.target.value;
    acao('modo', async () => {
      await go().TrocarModo(modo);
      s.modoTrocado = true;
    });
    return;
  }
  const campo = evento.target.closest('[data-pref]');
  if (!campo) return;
  salvarPreferencias({ [campo.dataset.pref]: campo.type === 'checkbox' ? campo.checked : campo.value });
});

document.addEventListener('keydown', (evento) => {
  if (evento.key === 'Escape') {
    if (s.pedidoFechar) acoes['fechar-cancelar']();
    else if (s.troca) acoes['fechar-troca']();
  }
  if (evento.key === 'Enter' && evento.target.matches('input[data-campo]')) {
    evento.preventDefault();
    acoes['proton-entrar']();
  }
});

document.addEventListener('input', (evento) => {
  const campo = evento.target.closest('[data-campo]');
  if (campo) s.proton[campo.dataset.campo] = campo.value;
});

window.runtime.EventsOn('arquivo-solto', (info) => escolher(info));
window.runtime.EventsOn('atualizar', () => atualizar());
window.runtime.EventsOn('proton-verificacao', () => {
  s.proton.etapa = 'verificacao';
  desenhar();
});
window.runtime.EventsOn('pedido-fechar', (temBandeja) => {
  s.pedidoFechar = { bandeja: Boolean(temBandeja) };
  desenhar();
});

aplicarTema();
go().Preferencias().then((prefs) => { s.prefs = prefs; aplicarTema(); desenhar(); }).catch(() => {});
atualizar().then(carregarCandidatos);
go().VersaoNova().then((tag) => { s.versaoNova = tag; desenhar(); }).catch(() => {});
setInterval(atualizar, 3000);
