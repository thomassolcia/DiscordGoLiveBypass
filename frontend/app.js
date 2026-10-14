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
};

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

// Pedaços da tela -------------------------------------------------------------------------

function avisos() {
  const app = s.app;
  let html = '';
  if (s.erro) html += `<div class="faixa erro"><span>${esc(frase(s.erro))}</span></div>`;
  if (app?.error) html += `<div class="faixa erro"><span>${esc(frase(app.error))}</span></div>`;
  if (app?.installed && !app.service_running) {
    html += `<div class="faixa atencao"><span>O serviço do túnel está instalado, mas parado.</span>
      <button data-acao="consertar" ${s.ocupado ? 'disabled' : ''}>Consertar</button></div>`;
  }
  if (app?.needs_update) {
    html += `<div class="faixa info"><span>Esta janela é da versão ${esc(app.version)} e o serviço instalado é da ${esc(app.status.version)}.</span>
      <button data-acao="atualizar" ${s.ocupado ? 'disabled' : ''}>${s.ocupado === 'atualizar' ? 'Atualizando…' : 'Atualizar'}</button></div>`;
  }
  if (s.versaoNova) {
    html += `<div class="faixa info"><span>Saiu a versão ${esc(s.versaoNova)}.</span>
      <button data-acao="baixar">${icone('externo')} Baixar</button></div>`;
  }
  return html;
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

function marcarAbrirDiscord() {
  const nome = s.app?.discord || 'Discord';
  return `<label class="marcar">
      <input type="checkbox" data-pref="abrir_discord" ${s.prefs.abrir_discord ? 'checked' : ''}>
      <span>Abrir o ${esc(nome)} quando o túnel ligar</span>
    </label>`;
}

function telaInicial() {
  const jaInstalado = s.app?.service_running;
  const pronto = s.escolhido && !s.escolhido.error;
  const ativando = s.ocupado === 'ativar';

  return `${avisos()}
    <div class="grade">
      <section class="apresentacao">
        <span class="rotulo">Go Live no Brasil</span>
        <h1>Seu Go Live de volta no Discord</h1>
        <p>Um túnel WireGuard leva só o Discord para fora do Brasil. O jogo que você transmite, o navegador e o resto do computador continuam na sua conexão.</p>
        <ul class="vantagens">
          <li><span class="icone">${icone('escudo')}</span><div><b>Só o Discord passa pela VPN</b><span>Nada de ping alto no jogo nem site estranhando seu IP.</span></div></li>
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
        <section class="cartao">
          <h2><span class="passo">1</span> Arquivo da VPN</h2>
          ${listaArquivos()}
          ${s.escolhido?.error ? `<div class="faixa erro"><span>${esc(frase(s.escolhido.error))}</span></div>` : ''}
          ${zonaSoltar('Arraste o arquivo <b>.conf</b> para cá')}
        </section>
        <section class="cartao">
          <h2><span class="passo">2</span> Ativar</h2>
          <p class="apagado">${jaInstalado
            ? 'O serviço do túnel já está instalado. É só ativar.'
            : 'O Windows vai pedir permissão de administrador uma única vez, para instalar o serviço que mantém o túnel ligado.'}</p>
          ${marcarAbrirDiscord()}
          <button class="principal grande largo" data-acao="ativar" ${pronto && !s.ocupado ? '' : 'disabled'}>
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

  switch (st.state) {
    case 'ligado':
      if (s.saida?.blocked) {
        titulo = 'O Discord ainda vê o Brasil';
        detalhe = 'O servidor escolhido não tirou você do Brasil. Troque o arquivo da VPN por outro servidor.';
        classe = 'alerta';
      } else {
        titulo = 'Go Live liberado';
        detalhe = 'Só o Discord está passando pelo túnel.';
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

  const reiniciar = s.avisoDiscord && app.discord_open.length
    ? `<div class="faixa atencao dentro"><span>O ${esc(app.discord_open.join(' e o '))} já estava aberto. Reinicie para ele entrar no túnel.</span>
        <button data-acao="reiniciar" ${s.ocupado ? 'disabled' : ''}>${icone('recarregar')} ${s.ocupado === 'reiniciar' ? 'Reiniciando…' : 'Reiniciar'}</button></div>`
    : '';

  const discord = app.discord
    ? `<button class="principal grande" data-acao="abrir-discord" ${s.ocupado ? 'disabled' : ''}>${icone('play', true)} Abrir o ${esc(app.discord)}</button>`
    : '';
  const alternar = st.enabled
    ? `<button class="grande" data-acao="desligar" ${s.ocupado ? 'disabled' : ''}>${icone('power')} ${s.ocupado === 'desligar' ? 'Desligando…' : 'Desligar'}</button>`
    : `<button class="principal grande" data-acao="ligar" ${s.ocupado ? 'disabled' : ''}>${icone('power')} ${s.ocupado === 'ligar' ? 'Ligando…' : 'Ligar o túnel'}</button>`;

  return `<section class="cartao estado ${classe}">
      <div class="topo">
        <span class="selo">${simbolo}</span>
        <div><div class="titulo">${esc(titulo)}</div><div class="detalhe">${detalhe}</div></div>
      </div>
      ${saida}
      ${reiniciar}
      <div class="acoes">${st.enabled ? discord + alternar : alternar}</div>
    </section>`;
}

function painel() {
  const st = s.app.status;
  const trocar = s.escolhido && !s.escolhido.error && s.escolhido.origin === 'escolhido';
  const handshake = st.handshake_s < 0 ? 'nenhum ainda' : `há ${st.handshake_s} s`;

  return `${avisos()}
    <div class="grade">
      <div class="coluna">
        ${cartaoEstado(st)}
        <section class="cartao">
          <h2>Preferências</h2>
          ${marcarAbrirDiscord()}
          <p class="pequeno">Minimizar esconde esta janela na bandeja, perto do relógio. O túnel não depende dela: fica ligado mesmo com ela fechada e volta sozinho quando o Windows inicia.</p>
        </section>
      </div>

      <div class="coluna">
        <section class="cartao">
          <h2>Servidor da VPN</h2>
          <div class="servidor">
            <span class="icone">${icone('servidor')}</span>
            <div><b>${esc(st.server || '-')}</b><small>${esc(st.endpoint || '')}</small></div>
          </div>
          ${trocar ? `<div class="faixa info"><span>Trocar para <b>${esc(s.escolhido.server)}</b>?</span>
            <span class="confirmar"><button class="principal" data-acao="trocar">Trocar</button>
            <button data-acao="cancelar-troca">Cancelar</button></span></div>` : ''}
          ${s.escolhido?.error ? `<div class="faixa erro"><span>${esc(frase(s.escolhido.error))}</span></div>` : ''}
          ${zonaSoltar('Para trocar de servidor, arraste outro <b>.conf</b>')}
        </section>
        <details class="cartao">
          <summary>Detalhes técnicos</summary>
          <div class="par"><span>Último handshake</span><b>${esc(handshake)}</b></div>
          <div class="par"><span>Recebido / enviado</span><b>${bytes(st.rx)} / ${bytes(st.tx)}</b></div>
          <div class="par"><span>Faixas do Discord no túnel</span><b>${st.routes}</b></div>
          <div class="par"><span>Versão do serviço</span><b>${esc(st.version)}</b></div>
        </details>
      </div>
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

function desenhar() {
  if (!s.app) return;
  document.getElementById('versao').textContent = s.app.version;
  tela.innerHTML = configurado() ? painel() : telaInicial();
  rodape.innerHTML = desenharRodape();
}

// Ações -------------------------------------------------------------------------------

const acoes = {
  escolher: async () => escolher(await go().EscolherArquivo()),
  proton: () => go().AbrirProton(),
  ajuda: () => go().AbrirAjuda(),
  baixar: () => go().AbrirRelease(),
  ativar: () => acao('ativar', async () => {
    const discordAberto = (s.app?.discord_open?.length || 0) > 0;
    await go().Ativar(s.escolhido.path);
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
  const tema = evento.target.closest('[data-tema]');
  if (tema) {
    salvarPreferencias({ tema: tema.dataset.tema });
    return;
  }
  const alvo = evento.target.closest('[data-acao], [data-arquivo]');
  if (!alvo || alvo.disabled) return;
  if (alvo.dataset.arquivo) {
    escolher([...s.candidatos, s.escolhido].find((c) => c && c.path === alvo.dataset.arquivo));
    return;
  }
  const fn = acoes[alvo.dataset.acao];
  if (fn) fn();
});

document.addEventListener('change', (evento) => {
  const campo = evento.target.closest('[data-pref]');
  if (campo) salvarPreferencias({ [campo.dataset.pref]: campo.checked });
});

window.runtime.EventsOn('arquivo-solto', (info) => escolher(info));
window.runtime.EventsOn('atualizar', () => atualizar());

aplicarTema();
go().Preferencias().then((prefs) => { s.prefs = prefs; aplicarTema(); desenhar(); }).catch(() => {});
atualizar().then(carregarCandidatos);
go().VersaoNova().then((tag) => { s.versaoNova = tag; desenhar(); }).catch(() => {});
setInterval(atualizar, 3000);
