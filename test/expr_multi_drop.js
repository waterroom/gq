// Drops one file onto the card of a *named* peer while several peers are
// visible, then reports which peer the transfer actually went to. This is the
// multi-user case: the drop must hit only the card under the cursor.
(async () => {
  const want = '接收端C';
  const cards = Array.from(document.querySelectorAll('.peer'));
  if (!cards.length) return { error: 'no peer card' };

  const names = cards.map((c) => c.querySelector('.peer-name').textContent.trim());
  const card = cards.find((c) => c.querySelector('.peer-name').textContent.trim() === want);
  if (!card) return { error: 'peer not found', names };

  const r = card.getBoundingClientRect();
  const cx = Math.round(r.left + r.width / 2);
  const cy = Math.round(r.top + r.height / 2);

  const bytes = new Uint8Array(64 * 1024);
  for (let i = 0; i < bytes.length; i++) bytes[i] = (i * 31) & 0xff;
  const dt = new DataTransfer();
  dt.items.add(new File([bytes], '多用户定向C.bin'));

  const mk = (t) =>
    new DragEvent(t, { bubbles: true, cancelable: true, dataTransfer: dt, clientX: cx, clientY: cy });

  window.dispatchEvent(mk('dragenter'));
  await new Promise((res) => setTimeout(res, 100));
  window.dispatchEvent(mk('dragover'));
  const highlighted = Array.from(document.querySelectorAll('.peer.drop-target')).map(
    (el) => el.querySelector('.peer-name').textContent.trim()
  );
  const title = document.querySelector('#dropTitle').textContent;
  const sub = document.querySelector('#dropSub').textContent;

  window.dispatchEvent(mk('drop'));
  await new Promise((res) => setTimeout(res, 2500));

  const st = await (await fetch('/api/state')).json();
  const t = st.transfers[0] || null;
  return {
    peersVisible: names,
    droppedOn: want,
    cardCenter: [cx, cy],
    highlighted,
    maskTitle: title,
    maskSub: sub,
    latest: t && {
      peer: t.peer,
      peerName: t.peer,
      state: t.state,
      done: t.done,
      total: t.total,
      files: (t.files || []).map((f) => f.name),
    },
    // every transfer the sender has recorded, to prove nothing went elsewhere
    allTransfers: st.transfers.map((x) => ({ peerName: x.peer, state: x.state, files: (x.files || []).map((f) => f.name) })),
  };
})()
