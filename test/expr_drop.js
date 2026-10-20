// Runs inside the page. Builds a real File, dispatches the same event sequence
// a human drag produces, and reports what the app did with it.
(async () => {
  const st0 = await (await fetch('/api/state')).json();

  const bytes = new Uint8Array(512 * 1024);
  for (let i = 0; i < bytes.length; i += 997) bytes[i] = i & 255;
  const file = new File([bytes], '注入拖拽.bin', { type: 'application/octet-stream' });

  const dt = new DataTransfer();
  dt.items.add(file);

  const mk = (t) =>
    new DragEvent(t, { bubbles: true, cancelable: true, dataTransfer: dt, clientX: 2, clientY: 2 });

  window.dispatchEvent(mk('dragenter'));
  await new Promise((r) => setTimeout(r, 120));
  const maskDuring = !document.querySelector('#dropMask').classList.contains('hidden');

  window.dispatchEvent(mk('dragover'));
  const highlighted = document.querySelectorAll('.peer.drop-target').length;

  window.dispatchEvent(mk('drop'));
  const maskAfter = !document.querySelector('#dropMask').classList.contains('hidden');

  await new Promise((r) => setTimeout(r, 3000));

  const st = await (await fetch('/api/state')).json();
  const t = st.transfers[0] || null;
  return {
    peers: st0.peers.map((p) => p.name),
    maskDuringDrag: maskDuring,
    cardsHighlighted: highlighted,
    maskHiddenAfterDrop: !maskAfter,
    transferCount: st.transfers.length,
    latest: t && {
      dir: t.dir,
      state: t.state,
      done: t.done,
      total: t.total,
      files: t.files.map((f) => f.name),
    },
  };
})()
