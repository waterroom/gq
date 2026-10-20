const $ = (s) => document.querySelector(s);

const esc = (s) =>
  String(s ?? '').replace(/[&<>"']/g, (c) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
  }[c]));

let currentOffer = null;
let toastTimer = null;
let peersCache = [];

function toast(msg) {
  const el = $('#toast');
  el.textContent = msg;
  el.classList.remove('hidden');
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => el.classList.add('hidden'), 2200);
}

async function api(path, opts) {
  try {
    const r = await fetch(path, opts);
    if (!r.ok) return { ok: false, error: 'HTTP ' + r.status };
    return await r.json();
  } catch (e) {
    console.error(path, e);
    return null;
  }
}

const post = (path, body) =>
  api(path, {
    method: 'POST',
    headers: body ? { 'Content-Type': 'application/json' } : {},
    body: body ? JSON.stringify(body) : undefined,
  });

function fmtSize(n) {
  if (!n) return '0 B';
  const u = ['B', 'KB', 'MB', 'GB', 'TB'];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024;
    i++;
  }
  const d = v >= 100 ? 0 : v >= 10 ? 1 : 2;
  return v.toFixed(d) + ' ' + u[i];
}

const fmtSpeed = (bps) => (bps > 0 ? fmtSize(bps) + '/s' : '');

function shortName(t) {
  const first = t.files && t.files[0] ? t.files[0].name : '—';
  const extra = t.files && t.files.length > 1 ? ` 等 ${t.files.length} 个文件` : '';
  return first + extra;
}

/* ---------------- peers ---------------- */

function renderPeers(peers) {
  peersCache = peers;
  const grid = $('#peerGrid');
  const seen = new Set();

  for (const p of peers) {
    seen.add(p.id);
    let el = grid.querySelector(`[data-id="${p.id}"]`);
    if (!el) {
      el = document.createElement('div');
      el.className = 'peer';
      el.dataset.id = p.id;
      el.innerHTML = `
        <div class="avatar"></div>
        <div class="peer-name"></div>
        <div class="peer-ip"></div>
        <div class="peer-actions">
          <button class="mini" data-act="file">发送文件</button>
          <button class="mini" data-act="dir">文件夹</button>
        </div>`;
      grid.appendChild(el);
    }
    const avatar = el.querySelector('.avatar');
    const initial = (p.name || '?').trim().charAt(0).toUpperCase();
    if (avatar.textContent !== initial) avatar.textContent = initial;
    el.querySelector('.peer-name').textContent = p.name || '未命名设备';
    el.querySelector('.peer-ip').textContent = p.addrLabel || p.addr || '';
    // The server qualifies the name with the address when two peers share one,
    // so the drag hint names the same device the card shows.
    el.dataset.label = p.label || p.name;
  }

  grid.querySelectorAll('.peer').forEach((el) => {
    if (!seen.has(el.dataset.id)) el.remove();
  });

  $('#peerCount').textContent = String(peers.length);
  $('#peerEmpty').classList.toggle('hidden', peers.length > 0);
  grid.classList.toggle('is-empty', peers.length === 0);
}

$('#peerGrid').addEventListener('click', async (e) => {
  const card = e.target.closest('.peer');
  if (!card) return;
  const btn = e.target.closest('[data-act]');
  const mode = btn ? btn.dataset.act : 'file';
  if (card.classList.contains('busy')) return;

  card.classList.add('busy');
  const r = await post(`/api/pick-send?peer=${encodeURIComponent(card.dataset.id)}&mode=${mode}`);
  card.classList.remove('busy');

  if (!r) return;
  if (r.ok) toast('已开始发送');
  else if (r.error) toast(r.error);
});

/* ---------------- transfers ---------------- */

function ensureTrans(t) {
  const list = $('#transList');
  let el = list.querySelector(`[data-tid="${t.id}"]`);
  if (el) return el;

  el = document.createElement('div');
  el.className = 'trans';
  el.dataset.tid = t.id;
  el.innerHTML = `
    <div class="trans-top">
      <span class="trans-dir"></span>
      <span class="trans-name"></span>
      <span class="trans-time"></span>
      <span class="trans-state"></span>
    </div>
    <div class="bar"><i></i></div>`;
  list.prepend(el);
  return el;
}

function updateTrans(el, t) {
  const dir = el.querySelector('.trans-dir');
  dir.textContent = t.dir === 'send' ? '发送' : '接收';
  dir.className = 'trans-dir ' + (t.dir === 'send' ? 'send' : 'recv');

  el.querySelector('.trans-name').textContent = shortName(t);
  el.querySelector('.trans-name').title = shortName(t);

  // The row has no space for the device, but in a room full of people "who was
  // this from" is the first question asked of the history.
  if (t.peer) el.title = `${t.dir === 'send' ? '发给' : '来自'} ${t.peer}`;

  const pct = t.total > 0 ? Math.min(100, (t.done / t.total) * 100) : 0;
  el.querySelector('.bar > i').style.width = pct.toFixed(1) + '%';

  const st = el.querySelector('.trans-state');
  el.className = 'trans ' + t.state;

  const d = new Date(t.started || Date.now());
  const hh = String(d.getHours()).padStart(2, '0');
  const mm = String(d.getMinutes()).padStart(2, '0');
  el.querySelector('.trans-time').textContent = `${hh}:${mm}`;

  let full = '';
  if (t.state === 'done') {
    full = fmtSize(t.total) + ' · 完成';
  } else if (t.state === 'error') {
    full = t.err || '失败';
  } else if (t.state === 'canceled') {
    full = '已取消';
  } else if (t.state === 'waiting') {
    full = '等待对方确认…';
  } else {
    const speed = fmtSpeed(t.speed);
    full = speed
      ? `${fmtSize(t.done)} / ${fmtSize(t.total)} · ${speed}`
      : `${fmtSize(t.done)} / ${fmtSize(t.total)}`;
  }
  st.textContent = full;
  st.title = full; // hover shows the untruncated text
}

function renderTransfers(list) {
  const wrap = $('#transList');
  const seen = new Set();
  for (const t of list) {
    seen.add(t.id);
    updateTrans(ensureTrans(t), t);
  }
  wrap.querySelectorAll('[data-tid]').forEach((el) => {
    if (!seen.has(el.dataset.tid)) el.remove();
  });
  $('#transEmpty').classList.toggle('hidden', list.length > 0);
  wrap.classList.toggle('is-empty', list.length === 0);
}

/* ---------------- inbound offer ---------------- */

function showOffer(o, queueLen) {
  currentOffer = o;
  $('#offerSender').textContent = o.sender || '未知设备';
  $('#offerAvatar').textContent = (o.sender || '?').trim().charAt(0).toUpperCase();

  // Address disambiguates two machines that happen to share a hostname.
  const meta = [o.addr, `${o.files.length} 个文件`, fmtSize(o.total)].filter(Boolean).join(' · ');
  const queued = queueLen > 1 ? `  ·  另有 ${queueLen - 1} 个请求排队` : '';
  $('#offerSummary').textContent = meta + queued;

  const ul = $('#offerFiles');
  ul.innerHTML = '';
  o.files.slice(0, 200).forEach((f) => {
    const li = document.createElement('li');
    li.innerHTML = `<span class="fname"></span><span class="fsize"></span>`;
    li.querySelector('.fname').textContent = f.name;
    li.querySelector('.fsize').textContent = fmtSize(f.size);
    ul.appendChild(li);
  });
  if (o.files.length > 200) {
    const li = document.createElement('li');
    li.textContent = `…另有 ${o.files.length - 200} 个文件`;
    ul.appendChild(li);
  }
  $('#offerModal').classList.remove('hidden');
}

function hideOffer() {
  currentOffer = null;
  $('#offerModal').classList.add('hidden');
}

$('#btnAccept').addEventListener('click', async () => {
  const id = currentOffer ? currentOffer.id : '';
  hideOffer();
  await post('/api/accept', { id });
});

$('#btnReject').addEventListener('click', async () => {
  const id = currentOffer ? currentOffer.id : '';
  hideOffer();
  await post('/api/reject', { id });
});

// Where the inbound files will land. The dialog shows it because "where did
// my file go" is the first question after accepting, and changing it here
// means it is changed before the transfer starts rather than after.
function renderRecvDir(dir) {
  const el = $('#offerDest');
  const text = dir || '—';
  el.textContent = text;
  el.title = text;
}

$('#btnOfferDir').addEventListener('click', async () => {
  const r = await post('/api/recvdir');
  if (!r) return;
  if (r.ok) {
    renderRecvDir(r.dir);
    toast('之后接收的文件会存到这里');
  } else if (r.error) {
    toast(r.error);
  }
});

/* ---------------- drag & drop ---------------- */

// A dropped file is read by the page and piped straight through this process
// to the peer. No path is needed, which is what lets the same code work in a
// WebView2 window and in the Edge fallback window.

let dragDepth = 0;

const dropMask = $('#dropMask');
const dropTitle = $('#dropTitle');
const dropSub = $('#dropSub');

const carriesFiles = (e) => {
  const t = e.dataTransfer;
  return !!t && Array.from(t.types || []).includes('Files');
};

function peerCardAt(x, y) {
  const el = document.elementFromPoint(x, y);
  return el ? el.closest('.peer') : null;
}

function setDropTarget(card) {
  document.querySelectorAll('.peer.drop-target').forEach((el) => {
    if (el !== card) el.classList.remove('drop-target');
  });
  if (card) card.classList.add('drop-target');
}

function hideDropMask() {
  dragDepth = 0;
  dropMask.classList.add('hidden');
  setDropTarget(null);
}

function dropHint(card) {
  if (card) {
    dropTitle.textContent = `发送给 ${card.dataset.label || card.querySelector('.peer-name').textContent}`;
    dropSub.textContent = '松开鼠标开始发送';
    return;
  }
  if (peersCache.length === 0) {
    dropTitle.textContent = '还没有发现设备';
    dropSub.textContent = '请先确认对方的程序已经运行，并允许了防火墙';
  } else if (peersCache.length === 1) {
    dropTitle.textContent = `发送给 ${peersCache[0].label || peersCache[0].name}`;
    dropSub.textContent = '松开鼠标开始发送';
  } else {
    dropTitle.textContent = '拖到某个设备卡片上';
    dropSub.textContent = '对准卡片松开，就会发给那一台';
  }
}

window.addEventListener('dragenter', (e) => {
  if (!carriesFiles(e)) return;
  e.preventDefault();
  dragDepth++;
  dropMask.classList.remove('hidden');
  dropHint(peerCardAt(e.clientX, e.clientY));
});

window.addEventListener('dragover', (e) => {
  if (!carriesFiles(e)) return;
  // Mandatory: without it the webview navigates to the dropped file and the
  // entire UI is replaced by its contents.
  e.preventDefault();
  e.dataTransfer.dropEffect = 'copy';
  const card = peerCardAt(e.clientX, e.clientY);
  setDropTarget(card);
  dropHint(card);
});

window.addEventListener('dragleave', (e) => {
  if (dragDepth === 0) return;
  dragDepth = Math.max(0, dragDepth - 1);
  if (dragDepth === 0) hideDropMask();
});

window.addEventListener('drop', (e) => {
  if (!carriesFiles(e)) return;
  e.preventDefault();
  e.stopPropagation();

  const card = peerCardAt(e.clientX, e.clientY);
  hideDropMask();

  const items = Array.from(e.dataTransfer.items || []);
  const hasDir = items.some((it) => {
    const entry = it.webkitGetAsEntry ? it.webkitGetAsEntry() : null;
    return entry && entry.isDirectory;
  });
  if (hasDir) {
    toast('暂不支持拖入文件夹，请点设备卡片上的「文件夹」按钮');
    return;
  }

  const files = Array.from(e.dataTransfer.files || []);
  if (!files.length) return;

  let peerId = card ? card.dataset.id : '';
  if (!peerId) {
    if (peersCache.length === 1) peerId = peersCache[0].id;
    else if (peersCache.length === 0) return toast('还没有发现设备');
    else return toast('请把文件拖到某个设备卡片上');
  }
  dropSend(peerId, files);
});

async function dropSend(peerId, files) {
  const metas = files.map((f) => ({ name: f.name, size: f.size }));

  const prep = await post('/api/drop-prepare', { peer: peerId, files: metas });
  if (!prep) return toast('发送失败');
  if (!prep.ok) return toast(prep.error || '对方没有接受');

  for (let i = 0; i < files.length; i++) {
    let r;
    try {
      r = await fetch(`/api/drop-file?job=${encodeURIComponent(prep.job)}&idx=${i}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/octet-stream' },
        body: files[i],
      });
    } catch (err) {
      console.error('drop-file', err);
      return toast('传输中断');
    }
    if (!r.ok) {
      let msg = '传输失败';
      try {
        msg = (await r.json()).error || msg;
      } catch (_) { /* response had no JSON body */ }
      return toast(msg);
    }
  }
  await post(`/api/drop-done?job=${encodeURIComponent(prep.job)}`);
  toast('发送完成');
}

/* ---------------- top bar ---------------- */

const nameInput = $('#nameInput');

nameInput.addEventListener('change', async () => {
  const v = nameInput.value.trim();
  if (!v) {
    nameInput.value = (await api('/api/state'))?.self?.name || '';
    return;
  }
  const r = await post('/api/name', { name: v });
  toast(r && r.ok ? '名称已更新' : '改名失败');
});

$('#btnRecvDir').addEventListener('click', async () => {
  const r = await post('/api/recvdir');
  if (r && r.ok) toast('接收目录已更新');
});

$('#btnOpenDir').addEventListener('click', () => post('/api/open-recv-dir'));

/* ---------------- firewall ---------------- */

// Windows firewall blocks inbound connections by default, and on a network it
// classifies as Public it does not even show its "allow access" prompt - the
// failure is completely silent: this machine can send, nobody can send to it.
// The rule needs administrator rights, so the button starts an elevated copy of
// this exe and then polls until the rule shows up.
const btnFirewall = $('#btnFirewall');

async function refreshFirewall() {
  const r = await api('/api/firewall-status');
  if (!r) return;
  btnFirewall.textContent = r.present ? '防火墙已放行' : '放行防火墙';
  btnFirewall.classList.toggle('done', !!r.present);
  btnFirewall.title = r.present
    ? '入站规则已存在：仅允许同网段的设备访问本程序'
    : '以管理员身份添加防火墙入站规则，让其他电脑能把文件发过来';
}

btnFirewall.addEventListener('click', async () => {
  const before = await api('/api/firewall-status');
  if (before && before.present) return toast('防火墙已放行');

  const r = await post('/api/firewall', {});
  if (!r) return toast('请求失败');
  if (!r.ok) return toast(r.error || '提权失败');
  if (r.present) return refreshFirewall();

  toast('请在系统弹窗中点"是"');
  for (let i = 0; i < 30; i++) {
    await new Promise((res) => setTimeout(res, 1000));
    const s = await api('/api/firewall-status');
    if (s && s.present) {
      refreshFirewall();
      return toast('已放行，其他电脑现在可以发文件给你');
    }
  }
  toast('未检测到规则；若取消了授权请重试，详见 lantransfer.log');
});

refreshFirewall();

$('#btnRescan').addEventListener('click', async () => {
  await post('/api/rescan');
  toast('正在扫描局域网…');
});

/* ---------------- firewall / discovery warning ---------------- */

const NO_PEER_ALERT_MS = 20000;
let emptySince = Date.now();
let bannerDismissed = false;
let modeBannerDismissed = false;

// When WebView2 is missing the UI opens in Edge/Chrome, which is easy to
// mistake for "the wrong app opened". Say why, and offer the one fix.
function renderWindowMode(self) {
  if (self.window !== 'browser' || modeBannerDismissed) return;
  const el = $('#modeBanner');
  if (!el.classList.contains('hidden')) return;
  const why = self.windowNote || 'WebView2 不可用';
  $('#modeBannerText').textContent = `当前运行在浏览器窗口中（${why}）。装上 WebView2 运行时后就是独立的应用窗口。`;
  el.classList.remove('hidden');
}

$('#modeBannerClose').addEventListener('click', () => {
  modeBannerDismissed = true;
  $('#modeBanner').classList.add('hidden');
});

$('#btnGetWebView2').addEventListener('click', async () => {
  const r = await post('/api/open-webview2-download');
  toast(r && r.ok ? '已在浏览器中打开下载页' : '打开下载页失败');
});

function showBanner(msg) {
  $('#bannerText').textContent = msg;
  $('#banner').classList.remove('hidden');
}

function hideBanner() {
  $('#banner').classList.add('hidden');
}

$('#bannerClose').addEventListener('click', () => {
  bannerDismissed = true;
  hideBanner();
});

/* ---------------- main loop ---------------- */

function render(s) {
  document.title = s.self.name + ' · gq';

  // Five addresses on a multi-homed box would blow up the header; show the
  // primary one and put the full list in the tooltip.
  const ips = s.self.ips || [];
  const primary = ips[0] || '未获取到局域网地址';
  const extra = ips.length > 1 ? ` 等 ${ips.length} 个地址` : '';
  const text = `${primary}${extra}  ·  端口 ${s.self.port}`;
  const ipsEl = $('#selfIps');
  if (ipsEl.textContent !== text) ipsEl.textContent = text;
  ipsEl.title = ips.length ? ips.join('\n') : '';

  if (document.activeElement !== nameInput) nameInput.value = s.self.name;
  renderRecvDir(s.self.recvDir);
  renderWindowMode(s.self);

  const peers = s.peers || [];
  renderPeers(peers);
  renderTransfers(s.transfers || []);

  // Nothing discovered for 20 s: the overwhelmingly common cause is a firewall
  // prompt that was never approved, and that failure is otherwise silent.
  if (peers.length > 0) {
    emptySince = Date.now();
    hideBanner();
  } else if (!bannerDismissed && Date.now() - emptySince > NO_PEER_ALERT_MS) {
    showBanner('20 秒内未发现任何设备。请确认：① 对方程序已运行；② 本机和对方的 Windows 防火墙都允许了本程序——本机可直接点顶栏「放行防火墙」。');
  }

  const queue = s.offers || [];
  const head = queue[0] || null;
  if (head) {
    if (!currentOffer || currentOffer.id !== head.id) showOffer(head, queue.length);
  } else if (currentOffer) {
    hideOffer();
  }
}

async function tick() {
  const s = await api('/api/state');
  if (s && s.self) render(s);
  setTimeout(tick, 600);
}

tick();

// Close the inbound dialog with Escape = reject, so a stray keypress never
// silently accepts 40 GB from a stranger.
document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape' && currentOffer) {
    $('#btnReject').click();
  }
});
