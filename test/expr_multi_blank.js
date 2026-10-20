// Multi-peer case, dropped on empty space rather than a card: the app must
// refuse and ask which device, never pick one for the user.
(async () => {
  const before = (await (await fetch('/api/state')).json()).transfers.length;

  // Bottom-right of the transfers panel: inside the app, deliberately not a card.
  const cx = Math.round(innerWidth * 0.86);
  const cy = Math.round(innerHeight * 0.86);
  const hit = document.elementFromPoint(cx, cy);

  const bytes = new Uint8Array(4096).fill(7);
  const dt = new DataTransfer();
  dt.items.add(new File([bytes], '空白处.bin'));

  const mk = (t) =>
    new DragEvent(t, { bubbles: true, cancelable: true, dataTransfer: dt, clientX: cx, clientY: cy });

  window.dispatchEvent(mk('dragenter'));
  await new Promise((r) => setTimeout(r, 80));
  window.dispatchEvent(mk('dragover'));
  const maskTitle = document.querySelector('#dropTitle').textContent;
  const maskSub = document.querySelector('#dropSub').textContent;
  const highlighted = document.querySelectorAll('.peer.drop-target').length;

  window.dispatchEvent(mk('drop'));
  await new Promise((r) => setTimeout(r, 900));

  const toastEl = document.querySelector('#toast');
  const after = (await (await fetch('/api/state')).json()).transfers.length;
  return {
    hitElement: hit && (hit.className || hit.tagName),
    highlighted,
    maskTitle,
    maskSub,
    toastShown: !!toastEl && !toastEl.classList.contains('hidden'),
    toastText: toastEl ? toastEl.textContent.trim() : '',
    transfersBefore: before,
    transfersAfter: after,
    createdTransfer: after > before,
  };
})()
