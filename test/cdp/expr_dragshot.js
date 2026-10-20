// Leaves the page in the middle of a drag so a screenshot can be taken.
// Returns without dispatching drop, so the overlay and the highlighted card
// stay on screen.
(async () => {
  const card = document.querySelector('.peer');
  if (!card) return { error: 'no peer card' };
  const r = card.getBoundingClientRect();
  const cx = Math.round(r.left + r.width / 2);
  const cy = Math.round(r.top + r.height / 2);

  const dt = new DataTransfer();
  dt.items.add(new File([new Uint8Array(1024)], '设计文档.pdf'));
  dt.items.add(new File([new Uint8Array(1024)], '波形数据.bin'));

  const mk = (t) =>
    new DragEvent(t, { bubbles: true, cancelable: true, dataTransfer: dt, clientX: cx, clientY: cy });

  window.dispatchEvent(mk('dragenter'));
  await new Promise((res) => setTimeout(res, 120));
  window.dispatchEvent(mk('dragover'));

  return {
    cardCenter: [cx, cy],
    highlighted: document.querySelectorAll('.peer.drop-target').length,
    title: document.querySelector('#dropTitle').textContent,
    sub: document.querySelector('#dropSub').textContent,
  };
})()
