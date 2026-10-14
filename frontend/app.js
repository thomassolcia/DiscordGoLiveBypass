'use strict';

// Cada função de window.go.main.App é um método do App em Go. Elas devolvem promessas, e
// um erro do lado do Go chega aqui como rejeição com o texto da mensagem.
const go = () => window.go.main.App;

const tela = document.getElementById('tela');
const rodape = document.getElementById('rodape');

const s = {
  app: null,
  candidatos: [],
  escolhido: null,
  saida: null,
  conferidoEm: 0,
  ocupado: '',
  erro: '',
  avisoDiscord: false,
  versaoNova: '',
  confirmandoRemocao: false,
};

const estados = {
  ligado: ['Ligado', 'ok'],
  conectando: ['Conectando ao servidor…', 'espera'],
  sem_resposta: ['Servidor sem resposta', 'alerta'],
  desligado: ['Desligado', 'desligado'],
  erro: ['Erro no túnel', 'ruim'],
};

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
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

function configurado() {
  const st = s.app?.status;
  return Boolean(s.app?.service_running && st?.has_conf);
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

function escolher(info) {
  if (!info || !info.path) return;
  s.escolhido = info;
  desenhar();
}

async function carregarCandidatos() {
  try {
    s.candidatos = await go().Candidatos();
    if (!s.escolhido && s.candidatos.length) s.escolhido = s.candidatos[0];
  } catch (e) {
    s.candidatos = [];
  }
  desenhar();
}

function avisos() {
  const app = s.app;
  let html = '';
  if (s.erro) html += `<div class="erro">${esc(frase(s.erro))}</div>`;
  if (app?.error) html += `<div class="erro">${esc(frase(app.error))}</div>`;
  if (app?.installed && !app.service_running) {
    html += `<div class="aviso"><span>O serviço do túnel está instalado, mas parado.</span>
      <button data-acao="consertar" ${s.ocupado ? 'disabled' : ''}>Consertar</button></div>`;
  }
  if (app?.needs_update) {
    html += `<div class="aviso"><span>Esta janela é da versão ${esc(app.version)} e o serviço instalado é da ${esc(app.status.version)}.</span>
      <button data-acao="atualizar" ${s.ocupado ? 'disabled' : ''}>Atualizar</button></div>`;
  }
  if (s.versaoNova) {
    html += `<div class="aviso"><span>Saiu a versão ${esc(s.versaoNova)}.</span>
      <button data-acao="baixar">Baixar</button></div>`;
  }
  return html;
}

function listaArquivos() {
  let html = '';
  const vistos = new Set();
  const itens = [...s.candidatos];
  if (s.escolhido && !itens.some((c) => c.path === s.escolhido.path)) itens.unshift(s.escolhido);

  for (const c of itens) {
    if (vistos.has(c.path)) continue;
    vistos.add(c.path);
    const origem = c.origin === '2.0' ? 'guardado pela versão 2.0'
      : c.origin === 'downloads' ? `nos Downloads, ${esc(c.modified)}` : esc(c.modified);
    const marcado = s.escolhido?.path === c.path ? 'escolhida' : '';
    html += `<button class="opcao ${marcado}" data-arquivo="${esc(c.path)}">
      <b>${esc(c.server || c.name)}</b><small>${esc(c.name)} · ${origem}</small></button>`;
  }
  if (s.escolhido?.error) html += `<div class="erro">${esc(frase(s.escolhido.error))}</div>`;
  return html;
}

function zonaSoltar(texto) {
  return `<div class="soltar" style="--wails-drop-target: drop">
    <p>${texto}</p>
    <div class="linha">
      <button data-acao="escolher">Escolher arquivo</button>
      <button class="link" data-acao="proton">Abrir a página do Proton</button>
    </div>
  </div>`;
}

function telaInicial() {
  const jaInstalado = s.app?.service_running;
  const pronto = s.escolhido && !s.escolhido.error;
  const ativando = s.ocupado === 'ativar';
  return `
    <section class="hero">
      <h1>Go Live de volta no Discord</h1>
      <p>Só o Discord passa pela VPN. O jogo que você transmite, o navegador e o resto do computador continuam na sua conexão.</p>
    </section>
    ${avisos()}
    <section class="cartao">
      <h2><span class="num">1</span> Arquivo da VPN</h2>
      ${listaArquivos()}
      ${zonaSoltar('Arraste o arquivo <b>.conf</b> para cá')}
      <details>
        <summary>Como conseguir um arquivo grátis</summary>
        <ol>
          <li>Crie uma conta gratuita no Proton VPN.</li>
          <li>Em account.protonvpn.com, abra <b>Downloads</b> e vá até <b>WireGuard configuration</b>.</li>
          <li>Escolha <b>Windows</b> e um servidor gratuito fora do Brasil.</li>
          <li>Clique em <b>Create</b> e baixe o arquivo. Ele aparece aqui sozinho.</li>
        </ol>
      </details>
    </section>
    <section class="cartao">
      <h2><span class="num">2</span> Ativar</h2>
      <p class="apagado">${jaInstalado
        ? 'O serviço do túnel já está instalado. É só ativar.'
        : 'O Windows vai pedir permissão de administrador só desta vez, para instalar o serviço que mantém o túnel ligado.'}</p>
      <button class="principal largo" data-acao="ativar" ${pronto && !s.ocupado ? '' : 'disabled'}>
        ${ativando ? 'Ativando…' : 'Ativar'}</button>
    </section>`;
}

function detalheEstado(st) {
  switch (st.state) {
    case 'ligado':
      if (!s.saida) return 'Conferindo o que o Discord vê…';
      if (s.saida.erro) return `Não consegui conferir a saída: ${esc(s.saida.erro)}`;
      if (s.saida.blocked) return 'O Discord ainda vê você no Brasil. Troque o servidor da VPN.';
      return `O Discord vê você em ${esc(s.saida.label)}`;
    case 'conectando':
      return 'Esperando o servidor da VPN responder.';
    case 'desligado':
      return 'O Discord sai pela sua conexão normal, sem Go Live.';
    default:
      return esc(frase(st.error));
  }
}

function painel() {
  const app = s.app;
  const st = app.status;
  const [titulo, classe] = estados[st.state] || [st.state, 'ruim'];
  const ligado = st.enabled;
  const trocar = s.escolhido && !s.escolhido.error && s.escolhido.origin === 'escolhido';
  const handshake = st.handshake_s < 0 ? 'nenhum ainda' : `há ${st.handshake_s} s`;

  let html = avisos();
  html += `<section class="cartao estado ${classe}">
      <div class="ponto"></div>
      <div><div class="titulo">${esc(titulo)}</div><div class="detalhe">${detalheEstado(st)}</div></div>
    </section>`;

  if (s.avisoDiscord && app.discord_open.length) {
    html += `<div class="aviso"><span>O ${esc(app.discord_open.join(' e o '))} já estava aberto. Reinicie para ele passar pelo túnel.</span>
      <button data-acao="reiniciar" ${s.ocupado ? 'disabled' : ''}>${s.ocupado === 'reiniciar' ? 'Reiniciando…' : 'Reiniciar o Discord'}</button></div>`;
  }

  html += `<button class="${ligado ? '' : 'principal'} largo" data-acao="${ligado ? 'desligar' : 'ligar'}" ${s.ocupado ? 'disabled' : ''}>
      ${ligado ? 'Desligar' : 'Ligar'}</button>
    <p></p>
    <section class="cartao">
      <h2>VPN</h2>
      <div class="par"><span>Servidor</span><b>${esc(st.server || '-')}</b></div>
      <div class="par"><span>Endereço</span><b>${esc(st.endpoint || '-')}</b></div>
      ${s.saida && !s.saida.erro ? `<div class="par"><span>IP que o Discord vê</span><b>${esc(s.saida.ip)}</b></div>` : ''}
      <p></p>
      ${trocar ? `<div class="aviso"><span>Trocar para ${esc(s.escolhido.server)}?</span>
        <span class="confirmar"><button class="principal" data-acao="trocar">Trocar</button>
        <button data-acao="cancelar-troca">Cancelar</button></span></div>` : ''}
      ${s.escolhido?.error ? `<div class="erro">${esc(frase(s.escolhido.error))}</div>` : ''}
      ${zonaSoltar('Para trocar de servidor, arraste outro <b>.conf</b> para cá')}
    </section>
    <details class="cartao">
      <summary>Detalhes técnicos</summary>
      <p></p>
      <div class="par"><span>Último handshake</span><b>${esc(handshake)}</b></div>
      <div class="par"><span>Recebido / enviado</span><b>${bytes(st.rx)} / ${bytes(st.tx)}</b></div>
      <div class="par"><span>Faixas do Discord</span><b>${st.routes}</b></div>
      <div class="par"><span>Versão do serviço</span><b>${esc(st.version)}</b></div>
    </details>`;
  return html;
}

function desenharRodape() {
  if (!s.app) return '';
  let html = `<span>versão ${esc(s.app.version)}</span>
    <button class="link" data-acao="ajuda">Ajuda</button>`;
  if (s.app.installed) {
    html += s.confirmandoRemocao
      ? `<span class="confirmar">Remover o serviço e o túnel?
          <button class="perigo" data-acao="remover">Remover</button>
          <button class="link" data-acao="cancelar-remocao">Cancelar</button></span>`
      : `<button class="link" data-acao="pedir-remocao">Desinstalar</button>`;
  }
  return html;
}

function desenhar() {
  if (!s.app) return;
  tela.innerHTML = configurado() ? painel() : telaInicial();
  rodape.innerHTML = desenharRodape();
}

const acoes = {
  escolher: async () => escolher(await go().EscolherArquivo()),
  proton: () => go().AbrirProton(),
  ajuda: () => go().AbrirAjuda(),
  baixar: () => go().AbrirRelease(),
  ativar: () => acao('ativar', async () => {
    const tinhaDiscord = (s.app?.discord_open?.length || 0) > 0;
    await go().Ativar(s.escolhido.path);
    s.avisoDiscord = tinhaDiscord;
    s.escolhido = null;
  }),
  ligar: () => acao('ligar', async () => {
    const tinhaDiscord = (s.app?.discord_open?.length || 0) > 0;
    await go().Ligar();
    s.avisoDiscord = tinhaDiscord;
  }),
  desligar: () => acao('desligar', () => go().Desligar()),
  trocar: () => acao('trocar', async () => {
    await go().TrocarArquivo(s.escolhido.path);
    s.escolhido = null;
    s.saida = null;
    s.conferidoEm = 0;
  }),
  'cancelar-troca': () => { s.escolhido = null; desenhar(); },
  reiniciar: () => acao('reiniciar', async () => {
    await go().ReiniciarDiscord();
    s.avisoDiscord = false;
  }),
  atualizar: () => acao('atualizar', () => go().Atualizar()),
  consertar: () => acao('consertar', () => go().Atualizar()),
  'pedir-remocao': () => { s.confirmandoRemocao = true; desenhar(); },
  'cancelar-remocao': () => { s.confirmandoRemocao = false; desenhar(); },
  remover: () => acao('remover', () => go().Desinstalar()),
};

document.addEventListener('click', (evento) => {
  const alvo = evento.target.closest('[data-acao], [data-arquivo]');
  if (!alvo || alvo.disabled) return;
  if (alvo.dataset.arquivo) {
    escolher([...s.candidatos, s.escolhido].find((c) => c && c.path === alvo.dataset.arquivo));
    return;
  }
  const fn = acoes[alvo.dataset.acao];
  if (fn) fn();
});

window.runtime.EventsOn('arquivo-solto', (info) => escolher(info));

atualizar().then(carregarCandidatos);
go().VersaoNova().then((tag) => { s.versaoNova = tag; desenhar(); }).catch(() => {});
setInterval(atualizar, 3000);
