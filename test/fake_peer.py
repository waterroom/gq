import http.server, json, threading, socket, time, os, hashlib
from urllib.parse import urlparse, parse_qs

OUT = r"D:/gq/test"
got = {}

class H(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"
    def log_message(self, *a): pass
    def _json(self, code, obj):
        b = json.dumps(obj).encode()
        self.send_response(code)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(b)))
        self.end_headers()
        self.wfile.write(b)
    def do_GET(self):
        if self.path.startswith('/api/info'):
            self._json(200, {"id":"cccccccccccccccc","name":"PY-PEER","port":45901,"os":"windows"})
        else:
            self._json(404, {})
    def do_POST(self):
        n = int(self.headers.get('Content-Length') or 0)
        if self.path.startswith('/api/offer'):
            body = json.loads(self.rfile.read(n))
            files = body.get('files', [])
            print('OFFER files=%d first=%s size=%s' % (len(files), files[0]['name'], files[0]['size']), flush=True)
            self._json(200, {"session": "sess1"})
        elif self.path.startswith('/api/upload'):
            q = parse_qs(urlparse(self.path).query)
            idx = int(q['idx'][0])
            buf = bytearray()
            remain = n
            while remain > 0:
                chunk = self.rfile.read(min(262144, remain))
                if not chunk: break
                buf += chunk
                remain -= len(chunk)
            fn = os.path.join(OUT, 'got_%d.bin' % idx)
            with open(fn, 'wb') as f:
                f.write(buf)
            got[idx] = len(buf)
            print('UPLOAD idx=%d bytes=%d md5=%s' % (idx, len(buf), hashlib.md5(buf).hexdigest()), flush=True)
            self.send_response(200)
            self.send_header('Content-Length', '0')
            self.end_headers()
        elif self.path.startswith('/api/done'):
            print('DONE', flush=True)
            self._json(200, {"ok": True})
        else:
            self._json(404, {})

def announce():
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM, socket.IPPROTO_UDP)
    msg = b'{"t":"hi","id":"cccccccccccccccc","name":"PY-PEER","port":45901,"os":"windows"}'
    for _ in range(4):
        s.sendto(msg, ('239.255.72.89', 45800))
        time.sleep(0.3)

srv = http.server.ThreadingHTTPServer(('0.0.0.0', 45901), H)
threading.Thread(target=srv.serve_forever, daemon=True).start()
announce()
print('READY', flush=True)
time.sleep(60)
