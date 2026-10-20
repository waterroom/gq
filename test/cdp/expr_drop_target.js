// Drops two files directly onto the peer card (real coordinates, not the
// corner), which exercises the "hit a card" branch instead of the
// single-peer fallback.
(async () => {
  const card = document.querySelector('.peer');
  if (!card) return { error: 'no peer card' };
  const r = card.getBoundingClientRect();
  const cx = Math.round(r.left + r.width / 2);
  const cy = Math.round(r.top + r.height / 2);

  const a = new Uint8Array(128 * 1024).fill(65);
  const b = new Uint8Array(96 * 1024).fill(66);
  const dt = new DataTransfer();
  dt.items.add(new File([a], '多文件A.bin'));
  dt.items.add(new File([b], '多文件B.bin'));

  const mk = (t) =>
    new DragEvent(t, { bubbles: true, cancelable: true, dataTransfer: dt, clientX: cx, clientY: cy });

  window.dispatchEvent(mk('dragenter'));
  await new Promise((res) => setTimeout(res, 120));
  window.dispatchEvent(mk('dragover'));
  const highlighted = document.querySelectorAll('.peer.drop-target').length;
  const title = document.querySelector('#dropTitle').textContent;

  window.dispatchEvent(mk('drop'));
  await new Promise((res) => setTimeout(res, 3000));

  const st = await (await fetch('/api/state')).json();
  const t = st.transfers[0] || null;
  return {
    cardCenter: [cx, cy],
    highlighted,
    title,
    latest: t && {
      state: t.state,
      done: t.done,
      total: t.total,
      files: t.files.map((f) => f.name),
    },
  };
})()
