/* MyPhotoBook_ROAD 화면  made by. Nevertheless_D */
'use strict';

const T = new URLSearchParams(location.search).get('t') || '';
const $ = (s, r = document) => r.querySelector(s);
const $$ = (s, r = document) => Array.from(r.querySelectorAll(s));
const esc = s => String(s ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
const N = n => Number(n || 0).toLocaleString('ko-KR');

function bytes(b) {
  b = Number(b || 0);
  if (b >= 1e9) return (b / 1e9).toFixed(1) + 'GB';
  if (b >= 1e6) return Math.round(b / 1e6) + 'MB';
  if (b >= 1e3) return Math.round(b / 1e3) + 'KB';
  return b + 'B';
}
function dur(s) { s = Math.round(s || 0); return Math.floor(s / 60) + ':' + String(s % 60).padStart(2, '0'); }
function eta(s) {
  if (s == null) return '';
  if (s < 50) return '곧 끝나요';
  const m = Math.round(s / 60);
  if (m < 60) return `약 ${m}분 남음`;
  return `약 ${Math.floor(m / 60)}시간 ${m % 60}분 남음`;
}
const WD = ['일', '월', '화', '수', '목', '금', '토'];

async function api(path, body) {
  const opt = { headers: { 'x-token': T } };
  if (body !== undefined) {
    opt.method = 'POST';
    opt.headers['content-type'] = 'application/json';
    opt.body = JSON.stringify(body);
  }
  const r = await fetch(path, opt);
  let j = null;
  try { j = await r.json(); } catch (e) { /* 빈 응답 */ }
  if (!r.ok) throw new Error((j && j.error) || `문제가 생겼어요 (${r.status})`);
  return j;
}

let toastTimer;
function toast(msg, err) {
  let el = $('.toast');
  if (!el) { el = document.createElement('div'); document.body.appendChild(el); }
  el.className = 'toast' + (err ? ' err' : '');
  el.textContent = msg;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => el.remove(), err ? 5200 : 3200);
}

const I = {
  check: '<svg viewBox="0 0 24 24" fill="none" stroke="#fff" stroke-width="3.2" stroke-linecap="round" aria-hidden="true"><path d="M5 12.5l4.5 4.5L19 7.5"/></svg>',
  okBig: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round" aria-hidden="true"><path d="M5 12.5l4.5 4.5L19 7.5"/></svg>',
  lock: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><rect x="5" y="10.5" width="14" height="10" rx="2"/><path d="M8.5 10.5V7.5a3.5 3.5 0 0 1 7 0v3"/></svg>',
  unlock: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" aria-hidden="true"><rect x="5" y="10.5" width="14" height="10" rx="2"/><path d="M8.5 10.5V7.5a3.5 3.5 0 0 1 7 0"/></svg>',
  cable: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M9 3v4M15 3v4M7 7h10v4a5 5 0 0 1-10 0z M12 16v5"/></svg>',
  again: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 12a8 8 0 1 1-2.3-5.6"/><path d="M20 4v4.5h-4.5"/></svg>',
  trash: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4 7h16M9.5 7V4.5h5V7M6.5 7l1 13h9l1-13M10 11v5.5M14 11v5.5"/></svg>',
  info: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" aria-hidden="true"><circle cx="12" cy="12" r="9"/><path d="M12 11v6M12 7.5v.5"/></svg>',
  warn: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><path d="M12 3.5l9.5 16.5h-19z"/><path d="M12 10v4.5M12 17v.5"/></svg>',
  photo: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true"><rect x="3" y="5" width="18" height="14" rx="2"/><circle cx="9" cy="10" r="1.6"/><path d="M3 17l5-5 4 4 3-3 6 5"/></svg>',
  video: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true"><rect x="3" y="5" width="18" height="14" rx="2"/><path d="M10 9.5v5l4.5-2.5z" fill="currentColor"/></svg>',
  back: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M15 5l-7 7 7 7"/></svg>',
};

// ------------------------------------------------------------------ 상태

const S = {
  settings: null, poll: null, items: [], counts: {}, newBytes: 0, rev: -1, itemsRev: -2, loadingItems: false,
  tab: 'new', kind: 'all', preview: true, sel: new Set(), anchor: null,
  screen: '', connectKey: '', listKey: '', jobKey: '', fails: 0, settingsOpen: false, del: null,
};

function applyTheme() {
  const a = S.settings && S.settings.appearance;
  if (a === 'light' || a === 'dark') document.documentElement.setAttribute('data-theme', a);
  else document.documentElement.removeAttribute('data-theme');
}

// ------------------------------------------------------------------ 위 막대

function renderTop() {
  const p = S.poll || {}, d = p.device || {};
  const chip = $('#devchip');
  if (d.state === 'ready' || d.state === 'loading' || (d.state === 'locked' && S.items.length)) {
    const extra = d.state === 'loading' ? `목록 불러오는 중 ${d.catalogPct || 0}%`
      : d.state === 'locked' ? '잠김' : `케이블 · ${N(S.counts.all || 0)}개`;
    chip.innerHTML = `<i class="dot${d.state === 'ready' ? '' : ' off'}"></i><b>${esc(d.name || 'iPhone')}</b><span>${esc(extra)}</span>`;
  } else {
    const t = d.state === 'locked' ? 'iPhone 잠김' : d.state === 'connecting' ? 'iPhone 연결 중' : '연결된 iPhone 없음';
    chip.innerHTML = `<i class="dot off"></i><span>${t}</span>`;
  }
  const pb = $('#previewBtn');
  pb.classList.toggle('on', S.preview);
  pb.setAttribute('aria-checked', String(S.preview));
  pb.querySelector('span').textContent = S.preview ? '미리보기 켬' : '미리보기 끔';
  const u = p.update || {}, pill = $('#updpill');
  if (u.status === 'ready' && u.installed) {
    pill.textContent = `새 버전 ${u.installed} 준비됨 · 다시 시작`;
    pill.classList.remove('hidden');
  } else if (u.status === 'downloading' || u.status === 'installing') {
    pill.textContent = `새 버전 받는 중 ${Math.round((u.progress || 0) * 100)}%`;
    pill.classList.remove('hidden');
  } else pill.classList.add('hidden');
}

// ------------------------------------------------------------------ 연결 화면

function renderConnect() {
  const d = (S.poll && S.poll.device) || {};
  const st = d.state || 'none';
  const key = st + '|' + d.helper;
  if (S.screen === 'connect' && S.connectKey === key) return;
  S.screen = 'connect'; S.connectKey = key; S.listKey = '';
  let title = 'iPhone을 케이블로 연결해 주세요', sub = '연결되면 바로 사진 목록을 불러와요', now = 1;
  if (st === 'connecting') { title = 'iPhone과 연결하는 중이에요'; sub = "iPhone에 '이 컴퓨터를 신뢰하겠습니까?'가 보이면 신뢰를 눌러 주세요"; now = 3; }
  if (st === 'locked') { title = 'iPhone 잠금을 풀어 주세요'; sub = '잠금을 풀면 바로 사진 목록을 불러와요'; now = 2; }
  if (st === 'unavailable') { title = 'iPhone 연결 기능을 켜지 못했어요'; sub = '앱을 다시 설치해 주세요. 보기 메뉴 › 진단 정보 보기에서 자세한 기록을 볼 수 있어요'; now = 0; }
  const step = (n, b, s) => `<div class="step${now === n ? ' now' : ''}"><span class="n">0${n}</span><b>${b}</b><span>${s}</span></div>`;
  $('#main').innerHTML = `
  <div class="connect">
    <svg width="360" height="170" viewBox="0 0 360 170" role="img" aria-label="iPhone을 케이블로 맥에 연결하는 그림" style="max-width:100%;height:auto">
      <rect x="150" y="18" width="200" height="128" rx="10" fill="var(--panel)" stroke="var(--ink)" stroke-width="3"/>
      <rect x="164" y="32" width="172" height="100" rx="4" fill="var(--accent-soft)"/>
      <rect x="196" y="50" width="22" height="22" rx="2" fill="var(--accent)"/><rect x="222" y="50" width="22" height="22" rx="2" fill="var(--accent)" opacity=".6"/><rect x="248" y="50" width="22" height="22" rx="2" fill="var(--accent)" opacity=".35"/>
      <rect x="196" y="76" width="22" height="22" rx="2" fill="var(--accent)" opacity=".35"/><rect x="222" y="76" width="22" height="22" rx="2" fill="var(--accent)"/><rect x="248" y="76" width="22" height="22" rx="2" fill="var(--accent)" opacity=".6"/>
      <path d="M130 146 H370 L362 156 H138Z" fill="var(--ink)"/>
      <rect x="18" y="40" width="58" height="112" rx="12" fill="var(--panel)" stroke="var(--ink)" stroke-width="3"/>
      <rect x="38" y="48" width="18" height="5" rx="2.5" fill="var(--ink)"/>
      <rect x="27" y="62" width="40" height="40" rx="3" fill="var(--hot)" opacity=".85"/>
      <path d="M47 152 V162 Q47 166 51 166 H96 Q120 166 120 142 V128 Q120 120 128 120 H150" fill="none" stroke="var(--accent)" stroke-width="4" stroke-linecap="round" stroke-dasharray="1 9"/>
    </svg>
    <div style="display:flex;flex-direction:column;gap:6px;align-items:center">
      <h1>${title}</h1>
      <span class="sub" style="display:flex;align-items:center;gap:10px">${st === 'connecting' ? '<i class="spin"></i>' : ''}${sub}</span>
    </div>
    <div class="steps">
      ${step(1, '케이블 연결', 'USB-C 또는 라이트닝 케이블로 맥과 이어 주세요.')}
      ${step(2, '잠금 풀기', '처음 연결할 때는 iPhone 잠금이 풀려 있어야 해요.')}
      ${step(3, "'신뢰' 누르기", '"이 컴퓨터를 신뢰하겠습니까?"가 뜨면 신뢰를 눌러 주세요.')}
    </div>
    <div class="tip">${I.unlock.replace('<svg', '<svg style="color:var(--accent-text)"')}
      <div><b>많이 옮길 때 팁 · 자동 잠금 끄기</b><span>iPhone이 잠기면 전송이 멈출 수 있어요. 설정 › 디스플레이 및 밝기 › 자동 잠금을 '안 함'으로 두면 끝까지 쉬지 않고 옮겨요. 잠겨서 멈춰도 잠금을 풀면 멈춘 곳부터 이어서 해요.</span></div>
    </div>
  </div>
  <div class="credit">made by. Nevertheless_D</div>`;
}

// ------------------------------------------------------------------ 메인 화면

function renderMainShell() {
  if (S.screen === 'main') return;
  S.screen = 'main'; S.connectKey = ''; S.listKey = '';
  $('#main').innerHTML = `
  <div class="filters">
    <div class="seg" id="tabs">
      <button type="button" data-tab="new"></button><button type="button" data-tab="done"></button><button type="button" data-tab="all"></button>
    </div>
    <div class="chips" id="kinds">
      <button type="button" data-kind="all">모두</button><button type="button" data-kind="p">사진</button>
      <button type="button" data-kind="v">동영상</button><button type="button" data-kind="l">Live Photo</button>
    </div>
    <div class="spacer"></div>
    <button type="button" class="linkbtn" id="selAll"></button>
  </div>
  <div id="notices"></div>
  <div class="body">
    <div class="scroller" id="scroller"></div>
    <div class="rail" id="rail"></div>
  </div>
  <div class="actbar" id="actbar"></div>
  <div class="credit">made by. Nevertheless_D</div>`;
  const sc = $('#scroller');
  sc.addEventListener('click', onListClick);
  sc.addEventListener('change', onListChange);
  sc.addEventListener('load', e => { if (e.target.tagName === 'IMG') e.target.classList.add('ok'); }, true);
  sc.addEventListener('error', e => { if (e.target.tagName === 'IMG') e.target.remove(); }, true);
  sc.addEventListener('scroll', () => requestAnimationFrame(spyRail), { passive: true });
  $('#tabs').addEventListener('click', e => { const b = e.target.closest('button'); if (b) { S.tab = b.dataset.tab; S.listKey = ''; renderList(); sc.scrollTop = 0; } });
  $('#kinds').addEventListener('click', e => { const b = e.target.closest('button'); if (b) { S.kind = b.dataset.kind; S.listKey = ''; renderList(); sc.scrollTop = 0; } });
  $('#selAll').addEventListener('click', () => {
    const v = visible();
    const pick = S.tab === 'done' ? v : v.filter(a => !a.done);
    const all = pick.length && pick.every(a => S.sel.has(a.id));
    pick.forEach(a => all ? S.sel.delete(a.id) : S.sel.add(a.id));
    syncSel();
  });
  $('#rail').addEventListener('click', e => {
    const b = e.target.closest('button[data-m]');
    if (!b) return;
    const sec = sc.querySelector(`[data-month="${b.dataset.m}"]`);
    if (sec) sc.scrollTo({ top: sec.offsetTop - sc.offsetTop, behavior: 'smooth' });
  });
  $('#actbar').addEventListener('click', onBarClick);
  $('#actbar').addEventListener('change', e => {
    if (e.target.id === 'delAfter') {
      S.settings.delAfter = e.target.checked;
      api('/api/settings', { delAfter: e.target.checked }).catch(err => toast(err.message, true));
    }
  });
}

function visible() {
  return S.items.filter(a => (S.tab === 'all' || (S.tab === 'new' ? !a.done : a.done)) && (S.kind === 'all' || a.k === S.kind));
}

function dayKey(d) { return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`; }

function tileHTML(a) {
  const sel = S.sel.has(a.id);
  let badge = '';
  if (a.k === 'l') badge += '<span class="live">LIVE</span>';
  if (a.k === 'v') badge += `<span class="dur">${a.dur ? dur(a.dur) : '동영상'}</span>`;
  if (a.done) badge += '<span class="dimmed"></span><span class="moved">옮김</span>';
  const kind = a.k === 'l' ? 'Live Photo' : a.k === 'v' ? '동영상' : '사진';
  return `<button type="button" class="tile${sel ? ' sel' : ''}" data-id="${a.id}" aria-pressed="${sel}" aria-label="${esc(a.n)}, ${kind}${a.done ? ', 이미 옮김' : ''}">` +
    `<span class="ph">${a.k === 'v' ? I.video : I.photo}</span><img loading="lazy" decoding="async" alt="" src="/api/thumb?id=${a.t}&t=${T}">` +
    `${badge}<span class="chk">${I.check}</span></button>`;
}

function rowHTML(a) {
  const kind = a.k === 'l' ? 'Live Photo' : a.k === 'v' ? '동영상' : '사진';
  return `<div class="row${a.done ? ' done' : ''}"><input type="checkbox" data-id="${a.id}" aria-label="${esc(a.n)} 선택"${S.sel.has(a.id) ? ' checked' : ''}>` +
    `<span class="nm">${esc(a.n)}</span><span>${kind}</span><span class="muted">${bytes(a.s)}</span><span class="st">${a.done ? '이미 옮김' : '새 항목'}</span></div>`;
}

function renderList() {
  if (S.screen !== 'main') return;
  const c = S.counts || {};
  const tabs = { new: `새 항목 ${N(c.new)}`, done: `이미 옮김 ${N(c.done)}`, all: `전체 ${N(c.all)}` };
  $$('#tabs button').forEach(b => { b.textContent = tabs[b.dataset.tab]; b.classList.toggle('on', b.dataset.tab === S.tab); });
  $$('#kinds button').forEach(b => b.classList.toggle('on', b.dataset.kind === S.kind));
  renderNotices();
  const list = visible();
  const key = [S.tab, S.kind, S.preview, S.itemsRev, list.length].join('|');
  if (key === S.listKey) { renderBar(); return; }
  S.listKey = key;
  const sc = $('#scroller');
  const top = sc.scrollTop;
  if (!list.length) {
    const d = S.poll && S.poll.device || {};
    const msg = d.state === 'loading' ? ['사진 목록을 불러오는 중이에요', `${N(S.items.length)}개를 찾았어요`]
      : S.tab === 'new' && S.items.length ? ['새로 옮길 사진이 없어요', '모두 이미 맥에 옮겨 두었어요']
      : S.tab === 'done' ? ['아직 옮긴 사진이 없어요', '사진을 고르고 가져오기를 눌러 보세요']
      : ['보여 줄 사진이 없어요', 'iPhone 사진 앱에 사진이 있는지 확인해 주세요'];
    sc.innerHTML = `<div class="empty">${d.state === 'loading' ? '<i class="spin"></i>' : ''}<b>${msg[0]}</b><span>${msg[1]}</span></div>`;
    $('#rail').innerHTML = '';
    renderBar();
    return;
  }
  const out = [];
  let curDay = null, curMonth = null, buf = [], cnt = 0, newCnt = 0, d0 = null;
  const flush = () => {
    if (!buf.length) return;
    const wd = WD[d0.getDay()];
    const cLabel = S.tab === 'new' ? `${N(newCnt)}개 새 항목` : S.tab === 'done' ? `${N(cnt)}개` : newCnt ? `${N(newCnt)}개 새 항목` : `${N(cnt)}개 · 모두 옮김`;
    out.push(`<section class="sec" data-day="${curDay}" data-month="${curMonth}"><div class="date"><span class="d">${d0.getDate()}</span>` +
      `<span class="s">${d0.getMonth() + 1}월 · ${wd}</span><span class="c">${cLabel}</span><button type="button" data-day="${curDay}">이 날 모두</button></div>` +
      (S.preview ? `<div class="grid">${buf.map(tileHTML).join('')}</div>`
        : `<div class="list"><div class="in"><div class="row head"><span></span><span>이름</span><span>종류</span><span>크기</span><span>상태</span></div>${buf.map(rowHTML).join('')}</div></div>`) +
      `</section>`);
    buf = []; cnt = 0; newCnt = 0;
  };
  const months = [];
  for (const a of list) {
    const d = new Date(a.d * 1000);
    const dk = dayKey(d), mk = dk.slice(0, 7);
    if (dk !== curDay) {
      flush();
      if (mk !== curMonth) {
        out.push(`<div class="month">${d.getFullYear()}년 ${d.getMonth() + 1}월</div>`);
        months.push(mk);
      }
      curDay = dk; curMonth = mk; d0 = d;
    }
    buf.push(a); cnt++; if (!a.done) newCnt++;
  }
  flush();
  sc.innerHTML = out.join('');
  sc.scrollTop = top;
  // 연 · 월 이동 막대
  let rail = '', y = '';
  for (const m of months) {
    const [yy, mm] = m.split('-');
    if (yy !== y) { rail += `<span class="y">${yy}</span>`; y = yy; }
    rail += `<button type="button" data-m="${m}">${Number(mm)}월</button>`;
  }
  $('#rail').innerHTML = rail;
  spyRail();
  renderBar();
}

function spyRail() {
  const sc = $('#scroller');
  if (!sc) return;
  const top = sc.getBoundingClientRect().top + 8;
  let cur = null;
  for (const s of sc.querySelectorAll('.sec')) {
    const r = s.getBoundingClientRect();
    if (r.bottom > top) { cur = s.dataset.month; break; }
  }
  $$('#rail button').forEach(b => b.classList.toggle('on', b.dataset.m === cur));
}

function renderNotices() {
  const d = (S.poll && S.poll.device) || {};
  const dest = (S.poll && S.poll.dest) || {};
  const n = [];
  if (!S.preview) n.push('<div class="notice">미리보기를 끄면 사진을 그리지 않아서 목록이 훨씬 빨리 열려요.</div>');
  if (d.state === 'locked') n.push('<div class="notice warn">iPhone이 잠겼어요. 잠금을 풀면 계속돼요.</div>');
  if (d.state === 'loading') n.push(`<div class="notice">iPhone 사진 목록을 불러오는 중이에요 · ${d.catalogPct || 0}%</div>`);
  if (dest.available === false) n.push('<div class="notice warn">저장할 디스크가 연결되어 있지 않아요. 디스크를 연결하거나 설정에서 저장 위치를 바꿔 주세요.</div>');
  const html = n.join('');
  const box = $('#notices');
  if (box && box.innerHTML !== html) box.innerHTML = html;
}

function renderBar() {
  const bar = $('#actbar');
  if (!bar) return;
  const c = S.counts || {}, dest = (S.poll && S.poll.dest) || {};
  let n = 0, size = 0, doneN = 0;
  for (const a of S.items) if (S.sel.has(a.id)) { n++; size += a.s; if (a.done) doneN++; }
  const line1 = n ? `${N(n)}개 선택 · ${bytes(size)}${doneN ? ` (이미 옮김 ${N(doneN)}개 포함)` : ''}`
    : c.new ? `새 항목 ${N(c.new)}개 · ${bytes(S.newBytes)}` : '새로 옮길 항목이 없어요';
  const line2 = dest.available === false ? '저장할 디스크가 연결되어 있지 않아요' : `${dest.label || '사진 › MyPhotoBook'} › 연 › 월 › 일`;
  let goLabel = n ? '가져오기' : '새 항목 모두 가져오기', goOff = false;
  if (n && doneN === n) { goLabel = '이미 옮긴 항목이에요'; goOff = true; }
  if (!n && !c.new) goOff = true;
  const ready = S.poll && S.poll.device && S.poll.device.state === 'ready';
  const html = `<div class="info"><b>${line1}</b><span title="${esc(dest.path || '')}">${esc(line2)}</span></div>` +
    `<button type="button" class="small" data-act="dest">폴더 변경</button>` +
    `<label><input type="checkbox" id="delAfter"${S.settings && S.settings.delAfter ? ' checked' : ''}>확인 후 iPhone에서 지우기</label>` +
    `<div class="spacer"></div>` +
    (n ? `<button type="button" class="del" data-act="delete"${ready ? '' : ' disabled'}>${I.trash}선택 항목 지우기</button>` : '') +
    `<button type="button" class="go" data-act="import"${goOff || !ready ? ' disabled' : ''}>${goLabel}</button>`;
  if (bar.dataset.html !== html) { bar.innerHTML = html; bar.dataset.html = html; }
  const all = visible().filter(a => S.tab === 'done' || !a.done);
  const selAll = $('#selAll');
  if (selAll) {
    const on = all.length && all.every(a => S.sel.has(a.id));
    selAll.textContent = on ? '선택 모두 풀기' : (S.tab === 'done' ? '모두 선택' : '새 항목 모두 선택');
    selAll.disabled = !all.length;
  }
}

function syncSel() {
  $$('#scroller .tile').forEach(t => { const on = S.sel.has(t.dataset.id); t.classList.toggle('sel', on); t.setAttribute('aria-pressed', String(on)); });
  $$('#scroller input[data-id]').forEach(i => { i.checked = S.sel.has(i.dataset.id); });
  renderBar();
}

function onListClick(e) {
  const dayBtn = e.target.closest('button[data-day]');
  if (dayBtn) {
    const day = dayBtn.dataset.day;
    const pick = visible().filter(a => dayKey(new Date(a.d * 1000)) === day && (S.tab === 'done' || !a.done));
    const all = pick.length && pick.every(a => S.sel.has(a.id));
    pick.forEach(a => all ? S.sel.delete(a.id) : S.sel.add(a.id));
    syncSel();
    return;
  }
  const t = e.target.closest('.tile');
  if (!t) return;
  const id = t.dataset.id;
  if (e.shiftKey && S.anchor) {
    const v = visible().map(a => a.id);
    const i = v.indexOf(S.anchor), j = v.indexOf(id);
    if (i >= 0 && j >= 0) {
      const on = S.sel.has(S.anchor);
      for (let k = Math.min(i, j); k <= Math.max(i, j); k++) on ? S.sel.add(v[k]) : S.sel.delete(v[k]);
      syncSel();
      return;
    }
  }
  S.sel.has(id) ? S.sel.delete(id) : S.sel.add(id);
  S.anchor = id;
  syncSel();
}

function onListChange(e) {
  const i = e.target.closest('input[data-id]');
  if (!i) return;
  i.checked ? S.sel.add(i.dataset.id) : S.sel.delete(i.dataset.id);
  S.anchor = i.dataset.id;
  renderBar();
}

async function chooseDest() {
  try {
    const dest = (S.poll && S.poll.dest) || {};
    const r = await api('/api/choose-folder', { start: dest.path || '' });
    if (!r.path) return;
    S.settings = await api('/api/settings', { customDest: r.path });
    toast(`이제 '${S.settings.dest.label}' 안에 정리해요`);
    await poll(true);
    if (S.settingsOpen) renderSettings();
  } catch (err) { toast(err.message, true); }
}

async function onBarClick(e) {
  const b = e.target.closest('button[data-act]');
  if (!b || b.disabled) return;
  const act = b.dataset.act;
  if (act === 'dest') return chooseDest();
  if (act === 'import') {
    b.disabled = true;
    try {
      const ids = [...S.sel];
      await api('/api/import', ids.length ? { ids, askDelete: !!S.settings.delAfter } : { all: true, askDelete: !!S.settings.delAfter });
      S.sel.clear();
      syncSel();
      await poll(true);
    } catch (err) { toast(err.message, true); b.disabled = false; }
  }
  if (act === 'delete') openDelete([...S.sel]);
}

// ------------------------------------------------------------------ 고른 사진 지우기 (⑤)

async function openDelete(ids) {
  if (!ids.length) return;
  try {
    const plan = await api('/api/delete/plan', { ids });
    S.del = { ids, plan, mode: 'import-first' };
    renderDelete();
  } catch (err) { toast(err.message, true); }
}

function renderDelete() {
  const d = S.del;
  if (!d) { closeLayer('del'); return; }
  const c = d.plan.counts || {}, total = c.total || 0, on = c.onMac || 0, off = c.notOnMac || 0;
  const kinds = [c.p ? `사진 ${N(c.p)}개` : '', c.v ? `동영상 ${N(c.v)}개` : '', c.l ? `Live Photo ${N(c.l)}개` : ''].filter(Boolean).join(' · ');
  const legend = [
    on ? `<div><i style="background:var(--accent)"></i><span><b>맥에 옮기고 확인 끝 ${N(on)}개</b><span class="muted"> · 지워도 맥에 남아 있어요</span></span></div>` : '',
    off ? `<div><i style="background:var(--hot)"></i><span><b>아직 맥에 없는 ${N(off)}개</b><span class="muted"> · 그냥 지우면 맥에도 남지 않아요</span></span></div>` : '',
  ].join('');
  const choices = off ? `
    <div style="display:flex;flex-direction:column;gap:8px">
      <label class="choice${d.mode === 'import-first' ? ' on' : ''}"><input type="radio" name="dm" value="import-first"${d.mode === 'import-first' ? ' checked' : ''}><span><b>맥에 먼저 옮긴 뒤 지우기 (추천)</b><small>${N(off)}개를 옮기고 확인한 다음, ${N(total)}개 모두 지워요</small></span></label>
      <label class="choice${d.mode === 'direct' ? ' on' : ''}"><input type="radio" name="dm" value="direct"${d.mode === 'direct' ? ' checked' : ''}><span><b>옮기지 않고 바로 지우기</b><small>필요 없는 사진만 고르셨을 때 써요</small></span></label>
    </div>` : '';
  const label = !off ? `${N(total)}개 지우기` : d.mode === 'import-first' ? `옮긴 뒤 ${N(total)}개 지우기` : `${N(total)}개 바로 지우기`;
  openLayer('del', `
  <div class="card" role="dialog" aria-modal="true" aria-labelledby="delTitle">
    <div class="head"><span class="ico danger">${I.trash}</span><div><h2 id="delTitle">고른 ${N(total)}개를 iPhone에서 지울까요?</h2><span class="sub">${kinds} · 약 ${bytes(d.plan.bytes)}</span></div></div>
    <div class="legend">${legend}</div>
    ${choices}
    <span class="foot">지운 사진은 iPhone 사진 앱의 '최근 삭제된 항목'에 남을 수 있어요. 공간을 바로 비우려면 거기서도 지워 주세요.</span>
    <div class="btns"><button type="button" class="b2" data-act="cancel">취소</button><button type="button" class="bd" data-act="go">${label}</button></div>
  </div>`, async e => {
    const r = e.target.closest('input[name="dm"]');
    if (r) { d.mode = r.value; renderDelete(); return; }
    const b = e.target.closest('button[data-act]');
    if (!b) return;
    if (b.dataset.act === 'cancel') { S.del = null; closeLayer('del'); return; }
    if (b.dataset.act === 'go') {
      const mode = off ? d.mode : 'import-first';
      if (mode === 'direct' && off && !confirm(`맥에 없는 ${N(off)}개는 어디에도 남지 않아요. 정말 바로 지울까요?`)) return;
      b.disabled = true;
      try {
        await api('/api/delete', { ids: d.ids, mode });
        S.del = null; closeLayer('del');
        S.sel.clear(); syncSel();
        await poll(true);
      } catch (err) { toast(err.message, true); b.disabled = false; }
    }
  });
}

// ------------------------------------------------------------------ 층(모달)

const layers = {};
function openLayer(name, html, onClick) {
  let el = layers[name];
  if (!el) {
    el = document.createElement('div');
    el.className = 'scrim';
    el.dataset.layer = name;
    $('#layer').appendChild(el);
    layers[name] = el;
  }
  el.innerHTML = html;
  el.onclick = onClick;
  el.onchange = onClick;
  return el;
}
function closeLayer(name) {
  const el = layers[name];
  if (el) { el.remove(); delete layers[name]; }
}

// ------------------------------------------------------------------ 가져오기 창 (①~④)

function jobView(j) {
  const st = j.stage;
  const pct = j.bytesTotal ? Math.min(100, Math.floor(j.bytesDone / j.bytesTotal * 100)) : 0;
  const dirOf = c => c && c.dir ? c.dir.split('/').join(' / ') : '';
  if (st === 'running' || st === 'paused') {
    const paused = st === 'paused';
    const cur = j.cur;
    const curKind = cur ? (cur.kind === 'l' ? 'Live Photo · 사진과 영상을 같은 폴더에 함께' : cur.kind === 'v' ? '동영상' : '사진') : '';
    const del3 = j.autoDelete ? [`옮긴 뒤 고른 ${N(j.autoDelete)}개를 지워요`, '대기'] : j.askDelete ? ['모두 확인한 뒤 한 번 더 물어볼게요', '대기'] : ['이번에는 지우지 않아요', '—'];
    return {
      key: 'run|' + paused + '|' + !!cur,
      html: `
      <div class="card" role="dialog" aria-modal="true" aria-labelledby="jobTitle">
        <div style="display:flex;align-items:flex-end;justify-content:space-between;gap:12px">
          <div style="display:flex;flex-direction:column;gap:2px"><h2 id="jobTitle">${paused ? '잠시 멈췄어요' : '가져오는 중'}</h2><span class="sub" data-f="sub"></span></div>
          <span class="bigpct" data-f="pct"></span>
        </div>
        <div class="bar"><i data-f="bar"></i></div>
        <div>
          <div class="srow"><span class="k">01</span><span class="t"><b>맥으로 복사</b><span>원본 그대로 (HEIC, MOV, ProRAW 포함)</span></span><span class="v" data-f="copied"></span></div>
          <div class="srow"><span class="k">02</span><span class="t"><b>복사본 확인</b><span>크기와 내용을 원본과 하나씩 비교</span></span><span class="v" data-f="verified"></span></div>
          <div class="srow"><span class="k off">03</span><span class="t"><b>iPhone에서 지우기</b><span>${del3[0]}</span></span><span class="muted">${del3[1]}</span></div>
        </div>
        ${cur ? `<div class="curfile"><span class="th"><img alt="" data-f="curimg"></span><span class="t"><b data-f="curname"></b><span data-f="curkind"></span></span></div>` : ''}
        <div class="btns"><span class="foot left">진행 상황은 계속 저장돼요. 멈춰도 이어서 할 수 있어요.</span>
          ${paused ? '<button type="button" class="b1" data-act="resume">이어서 하기</button>' : '<button type="button" class="b2" data-act="pause">잠시 멈춤</button>'}
          <button type="button" class="b2" data-act="cancel">취소</button></div>
      </div>`,
      fill: el => {
        set(el, 'sub', `${bytes(j.bytesDone)} / ${bytes(j.bytesTotal)}${!paused && j.eta != null ? ' · ' + eta(j.eta) : ''}`);
        set(el, 'pct', pct + '%');
        const bar = el.querySelector('[data-f="bar"]'); if (bar) bar.style.width = pct + '%';
        set(el, 'copied', `${N(Math.min(j.copied, j.total))} / ${N(j.total)}`);
        set(el, 'verified', `${N(j.verified)} / ${N(j.total)}`);
        if (cur) {
          set(el, 'curname', cur.name);
          set(el, 'curkind', `${curKind} → ${dirOf(cur)}`);
          const img = el.querySelector('[data-f="curimg"]');
          const src = `/api/thumb?id=${cur.id}&t=${T}`;
          if (img && img.dataset.src !== src) { img.dataset.src = src; img.style.opacity = 0; img.onload = () => { img.style.opacity = 1; }; img.onerror = () => { img.style.opacity = 0; }; img.src = src; }
        }
      },
    };
  }
  if (st === 'locked' || st === 'waiting' || st === 'problem') {
    const auto = S.settings ? S.settings.autoResume : true;
    const title = st === 'locked' ? 'iPhone이 잠겨서 잠시 멈췄어요' : st === 'waiting' ? (j.reason && j.reason.includes('불러오는') ? 'iPhone 사진 목록을 불러오는 중이에요' : 'iPhone 연결이 끊겨서 잠시 멈췄어요') : '잠시 멈췄어요';
    const sub = st === 'locked' ? (auto ? '잠금을 풀면 멈춘 곳부터 알아서 이어서 해요' : "잠금을 푼 뒤 '이어서 하기'를 눌러 주세요")
      : st === 'waiting' ? (j.reason && j.reason.includes('불러오는') ? '다 불러오면 바로 이어서 해요' : '케이블을 다시 연결하면 멈춘 곳부터 이어서 해요') : esc(j.reason);
    return {
      key: 'stop|' + st + '|' + !!j.partial + '|' + (j.reason || ''),
      html: `
      <div class="card" role="dialog" aria-modal="true" aria-labelledby="jobTitle">
        <div class="head"><span class="ico hot">${st === 'locked' ? I.lock : st === 'waiting' ? I.cable : I.warn}</span><div><h2 id="jobTitle">${title}</h2><span class="sub">${sub}</span></div></div>
        <div style="display:flex;flex-direction:column;gap:6px">
          <div style="display:flex;justify-content:space-between;font-size:13px"><span class="muted" data-f="saved"></span><span class="num" style="font-weight:600" data-f="pct"></span></div>
          <div class="bar hot"><i data-f="bar"></i></div>
        </div>
        <div class="box">
          <span><b data-f="donebox"></b><span class="muted"> · 맥에 안전하게 있어요</span></span>
          ${j.partial ? '<span><b>받다 만 파일 1개</b><span class="muted" data-f="partial"></span></span>' : ''}
        </div>
        ${st === 'locked' ? `<div class="box line" style="flex-direction:row;align-items:flex-start;gap:10px">${I.info.replace('<svg', '<svg style="width:18px;height:18px;flex:0 0 18px;margin-top:2px;color:var(--accent-text)"')}<span>다음부터 멈추지 않게 하려면 iPhone 설정 › 디스플레이 및 밝기 › 자동 잠금을 '안 함'으로 바꿔 두세요.</span></div>` : ''}
        <div class="btns"><button type="button" class="b2" data-act="cancel">여기서 그만하기</button>
          <button type="button" class="b1" data-act="resume">${st === 'locked' ? '잠금 풀었어요 · 이어서 하기' : '이어서 하기'}</button></div>
      </div>`,
      fill: el => {
        set(el, 'saved', `${N(j.doneFiles)} / ${N(j.total)}개 저장됨`);
        set(el, 'pct', pct + '%');
        const bar = el.querySelector('[data-f="bar"]'); if (bar) bar.style.width = pct + '%';
        set(el, 'donebox', `저장 완료 ${N(j.doneFiles)}개`);
        if (j.partial) set(el, 'partial', ` · ${j.partial.name} ${bytes(j.partial.size)} 중 ${bytes(j.partial.got)}까지 받음 → 거기서부터 이어 받아요`);
      },
    };
  }
  if (st === 'interrupted') {
    const when = j.stopAt ? new Date(j.stopAt * 1000) : null;
    const whenTxt = when ? `${when.getMonth() + 1}월 ${when.getDate()}일 ${when.getHours() < 12 ? '오전' : '오후'} ${((when.getHours() + 11) % 12) + 1}:${String(when.getMinutes()).padStart(2, '0')}에 멈춤` : '멈춤';
    const left = (j.total || 0) - (j.doneFiles || 0);
    return {
      key: 'int',
      html: `
      <div class="card" role="dialog" aria-modal="true" aria-labelledby="jobTitle">
        <div class="head"><span class="ico ok">${I.again}</span><div><h2 id="jobTitle">지난번 가져오기가 끝나지 않았어요</h2><span class="sub">${whenTxt} · ${esc(j.reason || '')}</span></div></div>
        <div class="stats">
          <div><b>${N(j.doneFiles)}</b><span>옮기고 확인 끝</span></div>
          <div><b class="ac">${N(left)}</b><span>남은 항목</span></div>
          ${j.partial ? `<div><b>1</b><span>받다 만 파일 (${bytes(j.partial.got)} 받음)</span></div>` : ''}
        </div>
        <span class="muted">이미 옮긴 ${N(j.doneFiles)}개는 건너뛰고${j.partial ? ', 받다 만 파일은 멈춘 곳부터 이어 받아요' : ' 남은 것만 옮겨요'}. 저장 위치(${esc(j.dest)})와 선택은 지난번 그대로예요.</span>
        <div class="btns"><button type="button" class="b2" data-act="discard">처음부터 다시 고르기</button><button type="button" class="b1" data-act="resume">이어서 하기</button></div>
      </div>`,
    };
  }
  if (st === 'deleting' || (st === 'done' && j.deleting)) {
    return {
      key: 'deleting',
      html: `<div class="card" role="dialog" aria-modal="true"><div class="head"><span class="ico danger"><i class="spin"></i></span><div><h2>iPhone에서 지우는 중이에요</h2><span class="sub">잠시만 기다려 주세요. 케이블을 빼지 마세요.</span></div></div></div>`,
    };
  }
  if (st === 'done') {
    const failed = j.failed || 0;
    if (j.kind === 'deleted' || j.kind === 'importDelete') {
      return {
        key: 'deleted|' + j.deleted,
        html: `
        <div class="card" role="dialog" aria-modal="true">
          <div class="head"><span class="ico ok">${I.okBig}</span><div><h2>iPhone에서 ${N(j.deleted || 0)}개를 지웠어요</h2><span class="sub">${j.kind === 'importDelete' ? '지우기 전에 맥에 먼저 옮기고 확인했어요' : '고른 항목을 지웠어요'}</span></div></div>
          ${j.deleteError ? `<div class="box" style="background:var(--hot-soft);color:var(--hot-text)">${esc(j.deleteError)}</div>` : ''}
          ${failed ? `<div class="box"><b>옮기지 못한 ${N(failed)}개는 지우지 않았어요</b>${(j.failedNames || []).slice(0, 4).map(x => `<span class="muted">${esc(x)}</span>`).join('')}</div>` : ''}
          <span class="foot">공간이 바로 늘지 않으면 iPhone 사진 앱의 '최근 삭제된 항목'을 비워 주세요.</span>
          <div class="btns">${j.destPath ? '<button type="button" class="bl left" data-act="reveal">폴더 열기</button>' : ''}<button type="button" class="b1" data-act="dismiss">확인</button></div>
        </div>`,
      };
    }
    const okAssets = j.deletable || 0;
    return {
      key: 'done|' + j.askDelete,
      html: `
      <div class="card" role="dialog" aria-modal="true">
        <div class="head"><span class="ico ${failed ? 'hot' : 'ok'}">${failed ? I.warn : I.okBig}</span><div><h2>${failed ? `${N(okAssets)}개를 옮겼어요 · ${N(failed)}개는 못 옮겼어요` : `${N(okAssets)}개를 안전하게 옮겼어요`}</h2><span class="sub">${esc(j.dest)} 안에 연 / 월 / 일로 정리했어요</span></div></div>
        <div class="stats">
          <div><b>${N(j.ok + j.skipped)}</b><span>복사</span></div>
          <div><b class="ac">${N(j.ok + j.skipped)}</b><span>확인 끝</span></div>
          <div><b>${N(j.livePairs)}</b><span>Live Photo 짝</span></div>
          <div><b>${N(failed)}</b><span>확인 실패</span></div>
        </div>
        ${failed ? `<div class="box">${(j.failedNames || []).slice(0, 5).map(x => `<span class="muted">${esc(x)}</span>`).join('')}<span>못 옮긴 항목은 iPhone에 그대로 있어요. 다시 가져오기를 누르면 그것만 옮겨요.</span></div>` : ''}
        ${j.askDelete ? `<div class="box line"><b style="font-size:15px">확인을 마친 ${N(okAssets)}개를 iPhone에서 지울까요?</b>
          <span class="muted">지우면 iPhone 공간이 약 ${bytes(j.bytes)} 늘어나요. 확인에 실패한 항목은 지우지 않아요.</span>
          <span class="muted">공간이 바로 늘지 않으면 iPhone 사진 앱의 '최근 삭제된 항목'을 비워 주세요.</span></div>` : ''}
        <div class="btns"><button type="button" class="bl left" data-act="reveal">폴더 열기</button>
          ${j.askDelete ? '<button type="button" class="b2" data-act="dismiss">지우지 않기</button><button type="button" class="bd" data-act="delVerified">iPhone에서 지우기</button>' : '<button type="button" class="b1" data-act="dismiss">확인</button>'}</div>
      </div>`,
    };
  }
  return null;
}

function set(el, f, txt) { const x = el.querySelector(`[data-f="${f}"]`); if (x && x.textContent !== txt) x.textContent = txt; }

function renderJob() {
  const j = (S.poll && S.poll.job) || {};
  const v = j.stage ? jobView(j) : null;
  if (!v) { if (S.jobKey) { closeLayer('job'); S.jobKey = ''; } return; }
  let el = layers.job;
  if (!el || S.jobKey !== v.key) {
    el = openLayer('job', v.html, onJobClick);
    S.jobKey = v.key;
  }
  if (v.fill) v.fill(el);
}

async function onJobClick(e) {
  const b = e.target.closest('button[data-act]');
  if (!b || b.disabled) return;
  const act = b.dataset.act;
  try {
    if (act === 'pause') await api('/api/job/pause', {});
    if (act === 'resume') await api('/api/job/resume', {});
    if (act === 'cancel') {
      if (!confirm('가져오기를 그만할까요? 이미 옮긴 사진은 맥에 그대로 있어요.')) return;
      await api('/api/job/cancel', {});
    }
    if (act === 'discard') {
      if (!confirm('지난번 작업을 버릴까요? 이미 옮긴 사진은 맥에 그대로 있어요.')) return;
      await api('/api/job/discard', {});
    }
    if (act === 'dismiss') await api('/api/job/dismiss', {});
    if (act === 'delVerified') { b.disabled = true; await api('/api/job/delete', {}); }
    if (act === 'reveal') { await api('/api/reveal', { path: (S.poll.job && S.poll.job.destPath) || '' }); return; }
    await poll(true);
  } catch (err) { toast(err.message, true); b.disabled = false; }
}

// ------------------------------------------------------------------ 설정

function sw(key, on, label, sub) {
  return `<div class="trow"><span class="t"><b>${label}</b><span>${sub}</span></span><button type="button" class="sw" role="switch" aria-checked="${!!on}" aria-label="${label}" data-sw="${key}"></button></div>`;
}

function renderSettings() {
  const s = S.settings, d = s.dest || {}, u = (S.poll && S.poll.update) || {};
  let sheet = $('#sheet');
  if (!sheet) {
    sheet = document.createElement('div');
    sheet.className = 'sheet';
    sheet.id = 'sheet';
    $('#layer').appendChild(sheet);
    sheet.addEventListener('click', onSettingsClick);
    sheet.addEventListener('change', onSettingsClick);
  }
  const upStatus = {
    checking: '확인하는 중…', uptodate: '지금이 최신 버전이에요', available: `새 버전 ${u.latest && u.latest.version} 이 있어요`,
    downloading: `새 버전 받는 중 ${Math.round((u.progress || 0) * 100)}%`, installing: '설치하는 중…',
    ready: `새 버전 ${u.installed} 준비됨 · 다시 시작하면 바뀌어요`, error: u.error || '업데이트를 확인하지 못했어요',
  }[u.status] || '새 버전이 나오면 앱이 알아서 받아요';
  const upBtn = u.status === 'ready' ? '<button type="button" class="b1" data-act="restart">다시 시작</button>'
    : u.status === 'available' ? '<button type="button" class="b1" data-act="install">지금 받기</button>'
    : `<button type="button" class="b1" data-act="check"${u.status === 'checking' || u.status === 'downloading' ? ' disabled' : ''}>업데이트 확인</button>`;
  const scrollTop = sheet.querySelector('.scroll') ? sheet.querySelector('.scroll').scrollTop : 0;
  sheet.innerHTML = `
  <div class="bar2"><button type="button" class="ibtn" data-act="close" aria-label="닫기">${I.back}</button><h1>설정</h1></div>
  <div class="scroll"><div class="inner">
    <div class="sbox">
      <h3>저장 위치</h3>
      <label class="radio"><input type="radio" name="dest" value="default"${d.mode !== 'custom' ? ' checked' : ''}><span><b>기본 폴더</b><span class="muted"> · ${esc(d.defLabel)}</span></span></label>
      <div style="display:flex;align-items:center;gap:10px">
        <label class="radio" style="flex:1;min-width:0"><input type="radio" name="dest" value="custom"${d.mode === 'custom' ? ' checked' : ''}${d.custom ? '' : ' disabled'}><span style="min-width:0"><b>다른 폴더</b><span class="muted"> · ${d.custom ? esc(d.custom) + (d.customOK ? '' : ' (지금 찾을 수 없어요)') : '외장 디스크도 고를 수 있어요'}</span></span></label>
        <button type="button" class="b2" data-act="choose" style="height:34px;font-size:13px">폴더 고르기…</button>
      </div>
    </div>
    <div class="sbox">
      <h3>폴더 정리 · 연 / 월 / 일</h3>
      <div class="cols">
        <div class="tree">
          <div style="font-weight:600">MyPhotoBook</div><div style="padding-left:16px">└ 2026</div><div style="padding-left:32px">└ 10</div><div style="padding-left:48px">└ 27</div>
          <div style="padding-left:70px">IMG_4821.HEIC <span class="live">LIVE</span></div><div style="padding-left:70px">IMG_4821.MOV <span class="live">LIVE 짝</span></div><div style="padding-left:70px">IMG_4822.MOV <span class="muted">동영상</span></div>
        </div>
        <div class="notes">
          <span>사진과 동영상 모두 찍은 날짜로 폴더를 나눠요.</span>
          <span>찍은 날짜 정보가 없으면 파일이 만들어진 날짜를 써요.</span>
          <span>Live Photo는 사진과 영상을 같은 이름으로 나란히 둬요.</span>
          <span>같은 이름의 다른 파일이 이미 있으면 뒤에 _1을 붙여요.</span>
        </div>
      </div>
    </div>
    <div class="sbox" style="gap:0;padding-top:12px;padding-bottom:6px">
      <h3 style="margin-bottom:2px">가져오기</h3>
      ${sw('skipDone', s.skipDone, '이미 옮긴 항목 건너뛰기', '가져온 기록과 파일 크기를 확인해서 같은 사진은 다시 옮기지 않아요')}
      ${sw('keepLive', s.keepLive, 'Live Photo 사진과 영상을 함께 옮기기', '둘 중 하나만 옮겨지는 일이 없도록 한 묶음으로 다뤄요')}
      ${sw('askDelete', s.askDelete, '복사 확인 후 iPhone에서 지울지 묻기', '확인을 마친 항목만, 한 번 더 물어본 뒤에 지워요')}
      ${sw('resumeJobs', s.resumeJobs, '멈추면 다음에 이어서 하기', '진행 상황을 계속 저장해서, 앱을 다시 열면 멈춘 곳부터 해요')}
      ${sw('autoResume', s.autoResume, 'iPhone 잠금을 풀면 자동으로 이어서 하기', '전송 중에 iPhone이 잠기면 기다렸다가 풀리는 즉시 계속해요')}
      ${sw('previewDefault', s.previewDefault, '앱을 열 때 미리보기 켜기', '끄면 사진을 그리지 않고 목록으로 보여 줘서 훨씬 빨라요')}
      <div class="trow foot" style="padding:10px 0 12px">한 번에 옮길 수 있는 개수에는 제한이 없어요.</div>
    </div>
    <div class="cols" style="gap:14px">
      <div class="sbox" style="flex:1 1 300px">
        <h3>화면</h3>
        <div class="seg wide" id="appearance">
          <button type="button" data-ap="system" class="${s.appearance === 'system' ? 'on' : ''}">맥 설정 따름</button>
          <button type="button" data-ap="light" class="${s.appearance === 'light' ? 'on' : ''}">라이트</button>
          <button type="button" data-ap="dark" class="${s.appearance === 'dark' ? 'on' : ''}">다크</button>
        </div>
      </div>
      <div class="sbox" style="flex:1 1 300px;gap:8px">
        <h3>업데이트</h3>
        <div style="display:flex;align-items:center;justify-content:space-between;gap:10px">
          <div style="display:flex;flex-direction:column;min-width:0"><span class="num" style="font-weight:600">버전 ${esc(s.build)}</span><span class="foot">${esc(upStatus)}</span></div>
          ${upBtn}
        </div>
        <div style="display:flex;gap:14px;flex-wrap:wrap">
          <button type="button" class="bl" style="height:30px;font-size:13px" data-act="diag">진단 정보 보기</button>
          ${u.canRollback ? `<button type="button" class="bl" style="height:30px;font-size:13px" data-act="rollback">이전 버전(${esc(u.backupVersion)})으로 되돌리기</button>` : ''}
        </div>
        ${sw('autoUpdate', s.autoUpdate, '새 버전 자동으로 받기', '켜 두면 새 버전이 나왔을 때 알아서 받아 둬요')}
      </div>
    </div>
    <div class="foot" style="padding:6px 2px">made by. Nevertheless_D</div>
  </div></div>`;
  sheet.querySelector('.scroll').scrollTop = scrollTop;
}

async function onSettingsClick(e) {
  if (e.type === 'change') {
    const r = e.target.closest('input[name="dest"]');
    if (r) {
      try { S.settings = await api('/api/settings', { destMode: r.value }); await poll(true); renderSettings(); } catch (err) { toast(err.message, true); }
    }
    return;
  }
  const swb = e.target.closest('[data-sw]');
  if (swb) {
    const k = swb.dataset.sw, v = swb.getAttribute('aria-checked') !== 'true';
    swb.setAttribute('aria-checked', String(v));
    try { S.settings = await api('/api/settings', { [k]: v }); } catch (err) { toast(err.message, true); }
    return;
  }
  const ap = e.target.closest('[data-ap]');
  if (ap) {
    try { S.settings = await api('/api/settings', { appearance: ap.dataset.ap }); applyTheme(); renderSettings(); } catch (err) { toast(err.message, true); }
    return;
  }
  const b = e.target.closest('button[data-act]');
  if (!b || b.disabled) return;
  const act = b.dataset.act;
  try {
    if (act === 'close') return closeSettings();
    if (act === 'choose') return chooseDest();
    if (act === 'diag') { window.open(`/diag`, '_blank'); return; }
    if (act === 'check') { b.disabled = true; await api('/api/update/check', {}); }
    if (act === 'install') await api('/api/update/install', {});
    if (act === 'restart') return restartForUpdate();
    if (act === 'rollback') {
      if (!confirm('이전 버전으로 되돌릴까요? 되돌린 뒤에는 자동 업데이트가 꺼져요.')) return;
      await api('/api/update/rollback', {});
      return restartForUpdate();
    }
    await poll(true);
    renderSettings();
  } catch (err) { toast(err.message, true); }
}

function openSettings() { S.settingsOpen = true; renderSettings(); }
function closeSettings() { S.settingsOpen = false; const sh = $('#sheet'); if (sh) sh.remove(); }

async function restartForUpdate() {
  try {
    await api('/api/update/restart', {});
    toast('새 버전으로 다시 여는 중이에요…');
  } catch (err) { toast(err.message, true); }
}

// ------------------------------------------------------------------ 주기적으로 상태 받기

let pollTimer, itemsTimer, lastItemsLoad = 0;

async function loadItems() {
  if (S.loadingItems) return;
  S.loadingItems = true;
  try {
    const r = await api('/api/items');
    S.items = r.assets || [];
    S.counts = r.counts || {};
    S.newBytes = r.newBytes || 0;
    S.itemsRev = r.rev;
    const ids = new Set(S.items.map(a => a.id));
    for (const id of [...S.sel]) if (!ids.has(id)) S.sel.delete(id);
    lastItemsLoad = Date.now();
    render();
  } catch (err) { /* 다음에 다시 */ } finally { S.loadingItems = false; }
}

function render() {
  renderTop();
  const d = (S.poll && S.poll.device) || {};
  const showMain = d.state === 'ready' || d.state === 'loading' || (d.state === 'locked' && S.items.length > 0);
  if (showMain) { renderMainShell(); renderList(); } else renderConnect();
  renderJob();
  if (S.settingsOpen && S.poll && JSON.stringify(S.poll.update) !== S.lastUpd) { S.lastUpd = JSON.stringify(S.poll.update); renderSettings(); }
}

async function poll(now) {
  clearTimeout(pollTimer);
  try {
    S.poll = await api('/api/poll');
    S.fails = 0;
    closeLayer('lost');
    if (S.poll.rev !== S.rev) {
      S.rev = S.poll.rev;
      const wait = Math.max(0, 1200 - (Date.now() - lastItemsLoad));
      clearTimeout(itemsTimer);
      itemsTimer = setTimeout(loadItems, now ? 0 : wait);
    }
    render();
  } catch (err) {
    S.fails++;
    if (S.fails >= 3) openLayer('lost', '<div class="card" style="align-items:center;text-align:center"><i class="spin"></i><h2>앱과 연결이 잠시 끊겼어요</h2><span class="sub">다시 연결하는 중이에요…</span></div>');
  }
  const j = S.poll && S.poll.job;
  const busy = j && j.stage && j.stage !== 'done';
  pollTimer = setTimeout(poll, busy ? 600 : 1300);
}

// ------------------------------------------------------------------ 시작

window.addEventListener('error', e => { api('/api/clientlog', { items: [`${e.message} @ ${e.filename}:${e.lineno}`] }).catch(() => {}); });
window.addEventListener('unhandledrejection', e => { api('/api/clientlog', { items: [`promise: ${e.reason && e.reason.message || e.reason}`] }).catch(() => {}); });

document.addEventListener('keydown', e => {
  if (e.key === 'Escape') {
    if (S.del) { S.del = null; closeLayer('del'); return; }
    if (S.settingsOpen) { closeSettings(); return; }
    if (S.sel.size) { S.sel.clear(); syncSel(); }
  }
  if ((e.metaKey || e.ctrlKey) && e.key === 'a' && S.screen === 'main' && !S.settingsOpen && !e.target.closest('input')) {
    e.preventDefault();
    visible().filter(a => S.tab === 'done' || !a.done).forEach(a => S.sel.add(a.id));
    syncSel();
  }
});

async function init() {
  try {
    S.settings = await api('/api/settings');
  } catch (err) {
    setTimeout(init, 1000);
    return;
  }
  applyTheme();
  S.preview = S.settings.previewDefault !== false;
  $('#previewBtn').addEventListener('click', () => { S.preview = !S.preview; S.listKey = ''; renderTop(); renderList(); });
  $('#settingsBtn').addEventListener('click', openSettings);
  $('#updpill').addEventListener('click', () => {
    const u = S.poll && S.poll.update;
    if (u && u.status === 'ready') restartForUpdate();
  });
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => { S.listKey = ''; render(); });
  poll(true);
}
init();
