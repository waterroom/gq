"""Same drop path, but the body trickles in like a real multi-GB file would.

A fast localhost upload finishes inside one poll interval, which would hide a
progress bug. Throttling the read side makes the intermediate states visible.
"""
import json
import threading
import time
import urllib.request

A = 'http://127.0.0.1:45800'
PEER = 'bbbbbbbbbbbbbbbb'
TOTAL = 4 * 1024 * 1024
CHUNK = 64 * 1024


class Trickle:
    """File-like body that emits one chunk every 40 ms."""

    def __init__(self, total, chunk):
        self.left = total
        self.chunk = chunk

    def read(self, n=-1):
        if self.left <= 0:
            return b''
        time.sleep(0.04)
        k = min(self.chunk, self.left)
        self.left -= k
        return b'\x5a' * k


def post_json(url, obj):
    req = urllib.request.Request(url, data=json.dumps(obj).encode(), method='POST',
                                 headers={'Content-Type': 'application/json'})
    with urllib.request.urlopen(req, timeout=90) as r:
        return json.load(r)


prep = post_json(A + '/api/drop-prepare', {'peer': PEER, 'files': [{'name': '慢速.bin', 'size': TOTAL}]})
print('prepare ->', prep)
job, tid = prep['job'], prep['transfer']

t0 = time.time()
samples, stop = [], False


def watch():
    while not stop:
        try:
            with urllib.request.urlopen(A + '/api/state', timeout=2) as r:
                d = json.load(r)
            for t in d['transfers']:
                if t['id'] == tid:
                    samples.append((round(time.time() - t0, 2), t['done'], t['speed']))
        except Exception:
            pass
        time.sleep(0.2)


th = threading.Thread(target=watch)
th.start()

req = urllib.request.Request(A + '/api/drop-file?job=%s&idx=0' % job, data=Trickle(TOTAL, CHUNK),
                             method='POST',
                             headers={'Content-Type': 'application/octet-stream',
                                      'Content-Length': str(TOTAL)})
with urllib.request.urlopen(req, timeout=180) as r:
    res = json.load(r)
stop = True
th.join()
print('drop-file ->', res, '(%.2fs)' % (time.time() - t0))

prev = -1
for ts, done, speed in samples:
    if done != prev:
        print('   %.2fs  done=%8d  speed=%s' % (ts, done, speed))
        prev = done

post_json(A + '/api/drop-done?job=%s' % job, {})
print('done sent')
