// Tiny Chrome DevTools Protocol driver: load a page, optionally run one
// expression in it, optionally screenshot. Enough to exercise the drop logic
// with a real File object instead of a hand-rolled HTTP call.
//
// usage: node cdp.mjs [port] [url] [exprFile] [shotFile] [waitMs]
import { readFileSync, writeFileSync } from 'node:fs';

const port = Number(process.argv[2] || 9222);
const url = process.argv[3] || 'http://127.0.0.1:45800/';
const exprFile = process.argv[4] && process.argv[4] !== '-' ? process.argv[4] : '';
const shotFile = process.argv[5] && process.argv[5] !== '-' ? process.argv[5] : '';
const waitMs = Number(process.argv[6] || 2500);

const list = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
// Edge ships several page targets (sync dialog, extensions); pick the blank
// one we launched rather than whichever happens to be first.
const pages = list.filter((t) => t.type === 'page');
const page =
  pages.find((t) => t.url === 'about:blank') ||
  pages.find((t) => t.url.startsWith('http://127.0.0.1')) ||
  pages[0];
if (!page) throw new Error('no page target on port ' + port);

const ws = new WebSocket(page.webSocketDebuggerUrl);
await new Promise((res, rej) => {
  ws.onopen = res;
  ws.onerror = rej;
});

let seq = 1;
const pending = new Map();
ws.onmessage = (ev) => {
  const msg = JSON.parse(ev.data);
  if (msg.id && pending.has(msg.id)) {
    pending.get(msg.id)(msg);
    pending.delete(msg.id);
  }
};
const send = (method, params = {}) =>
  new Promise((res) => {
    const id = seq++;
    pending.set(id, res);
    ws.send(JSON.stringify({ id, method, params }));
  });

await send('Page.enable');
await send('Runtime.enable');
await send('Emulation.setDeviceMetricsOverride', {
  width: 1040, height: 720, deviceScaleFactor: 1, mobile: false,
});
await send('Page.navigate', { url });
await new Promise((r) => setTimeout(r, waitMs));

if (exprFile) {
  const expr = readFileSync(exprFile, 'utf8');
  const r = await send('Runtime.evaluate', {
    expression: expr,
    awaitPromise: true,
    returnByValue: true,
    userGesture: true,
  });
  const out = r.result?.result?.value;
  if (out !== undefined) console.log(JSON.stringify(out, null, 2));
  if (r.result?.exceptionDetails) {
    console.error('EXCEPTION:', JSON.stringify(r.result.exceptionDetails, null, 2));
  }
}

if (shotFile) {
  await new Promise((r) => setTimeout(r, 400));
  const shot = await send('Page.captureScreenshot', { format: 'png' });
  if (!shot.result?.data) {
    console.error('screenshot failed:', JSON.stringify(shot).slice(0, 400));
  } else {
    writeFileSync(shotFile, Buffer.from(shot.result.data, 'base64'));
    console.log('screenshot ->', shotFile);
  }
}

ws.close();
