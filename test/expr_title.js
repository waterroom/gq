// The transfer row carries the peer in its tooltip; nothing else in the row
// tells you which of two same-named senders it belongs to.
(async () => {
  const row = document.querySelector('.trans');
  if (!row) return { error: 'no transfer row' };
  return { title: row.title, visibleText: row.textContent.replace(/\s+/g, ' ').trim() };
})()
