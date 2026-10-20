// Reads the firewall button state after the page has had time to query it.
(async () => {
  await new Promise((r) => setTimeout(r, 2500));
  const b = document.querySelector('#btnFirewall');
  const st = await (await fetch('/api/firewall-status')).json();
  return {
    buttonText: b.textContent,
    classes: b.className,
    title: b.title,
    status: st,
    color: getComputedStyle(b).color,
    background: getComputedStyle(b).backgroundColor,
  };
})()
