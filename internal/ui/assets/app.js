const $ = (s) => document.querySelector(s);

const esc = (s) =>
  String(s ?? '').replace(/[&<>"']/g, (c) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
  }[c]));

let currentOffer = null;
let toastTimer = null;

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
    el.querySelector('.peer-ip').textContent = p.addr || '';
  }

  grid.querySelectorAll('.peer').forEach((el) => {
    if (!seen.has(el.dataset.id)) el.remove();
  });

  $('#peerCount').textContent = String(peers.length);
  $('#peerEmpty').classList.toggle('hidden', peers.length > 0);
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

$('#btnRescan').addEventListener('click', async () => {
  await post('/api/rescan');
  toast('正在扫描局域网…');
});

/* ---------------- firewall / discovery warning ---------------- */

const NO_PEER_ALERT_MS = 20000;
let emptySince = Date.now();
let bannerDismissed = false;

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
  document.title = s.self.name + ' · 局域网快传';

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

  const peers = s.peers || [];
  renderPeers(peers);
  renderTransfers(s.transfers || []);

  // Nothing discovered for 20 s: the overwhelmingly common cause is a firewall
  // prompt that was never approved, and that failure is otherwise silent.
  if (peers.length > 0) {
    emptySince = Date.now();
    hideBanner();
  } else if (!bannerDismissed && Date.now() - emptySince > NO_PEER_ALERT_MS) {
    showBanner('20 秒内未发现任何设备。请确认：① 对方程序已运行；② 本机和对方的 Windows 防火墙都允许了本程序（专用 + 公用网络）。');
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
