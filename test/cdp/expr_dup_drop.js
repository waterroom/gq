// Two devices with the identical name. Drops on the one whose label carries
// :46100 and proves the bytes land there, not on the same-named twin.
(async () => {
  const needle = ':46100';
  const cards = Array.from(document.querySelectorAll('.peer'));
  const labels = cards.map((c) => ({
    label: c.dataset.label,
    name: c.querySelector('.peer-name').textContent.trim(),
    ip: c.querySelector('.peer-ip').textContent.trim(),
  }));

  const card = cards.find((c) => (c.dataset.label || '').includes(needle));
  if (!card) return { error: 'no card labelled ' + needle, labels };

  const r = card.getBoundingClientRect();
  const cx = Math.round(r.left + r.width / 2);
  const cy = Math.round(r.top + r.height / 2);

  const dt = new DataTransfer();
  dt.items.add(new File([new Uint8Array(32768).fill(9)], '同名定向D.bin'));

  const mk = (t) =>
    new DragEvent(t, { bubbles: true, cancelable: true, dataTransfer: dt, clientX: cx, clientY: cy });

  window.dispatchEvent(mk('dragenter'));
  await new Promise((res) => setTimeout(res, 100));
  window.dispatchEvent(mk('dragover'));
  const maskTitle = document.querySelector('#dropTitle').textContent;
  const highlighted = Array.from(document.querySelectorAll('.peer.drop-target')).map(
    (el) => el.dataset.label
  );

  window.dispatchEvent(mk('drop'));
  await new Promise((res) => setTimeout(res, 2500));

  const st = await (await fetch('/api/state')).json();
  const latest = st.transfers[0] || null;
  return {
    cards: labels,
    maskTitle,
    highlighted,
    latest: latest && { peer: latest.peer, state: latest.state, done: latest.done, files: (latest.files || []).map((f) => f.name) },
  };
})()
