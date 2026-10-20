"""End-to-end test for the drag-and-drop path.

The browser is replaced by a plain HTTP client: POST the file bytes to
/api/drop-file exactly as fetch() does, and verify the receiver got the same
bytes. Also samples /api/state during the upload to prove progress is
reported per chunk rather than once at the end.
"""
import hashlib
import json
import os
import threading
import time
import urllib.request

A = 'http://127.0.0.1:45800'
B = 'http://127.0.0.1:45900'
PEER = 'bbbbbbbbbbbbbbbb'
SRC = r'D:\gq\test\测试文件_8MB.bin'
NAME = '拖拽测试.bin'


def post_json(url, obj):
    data = json.dumps(obj).encode()
    req = urllib.request.Request(url, data=data, method='POST',
                                headers={'Content-Type': 'application/json'})
    with urllib.request.urlopen(req, timeout=90) as r:
        return json.load(r)


def md5(path):
    h = hashlib.md5()
    with open(path, 'rb') as f:
        for chunk in iter(lambda: f.read(1 << 20), b''):
            h.update(chunk)
    return h.hexdigest()


size = os.path.getsize(SRC)
print('source: %d bytes md5=%s' % (size, md5(SRC)))

t0 = time.time()
prep = post_json(A + '/api/drop-prepare', {'peer': PEER, 'files': [{'name': NAME, 'size': size}]})
print('prepare ->', prep, '(%.2fs)' % (time.time() - t0))
if not prep.get('ok'):
    raise SystemExit('prepare failed')

job, tid = prep['job'], prep['transfer']

samples, stop = [], False


def watch():
    while not stop:
        try:
            with urllib.request.urlopen(A + '/api/state', timeout=2) as r:
                d = json.load(r)
            for t in d['transfers']:
                if t['id'] == tid:
                    samples.append((round(time.time() - t0, 2), t['done'], t['state']))
        except Exception:
            pass
        time.sleep(0.15)


th = threading.Thread(target=watch)
th.start()

with open(SRC, 'rb') as f:
    req = urllib.request.Request(A + '/api/drop-file?job=%s&idx=0' % job, data=f,
                                 method='POST',
                                 headers={'Content-Type': 'application/octet-stream',
                                          'Content-Length': str(size)})
    with urllib.request.urlopen(req, timeout=180) as r:
        res = json.load(r)
stop = True
th.join()
print('drop-file ->', res, '(%.2fs)' % (time.time() - t0))

done = post_json(A + '/api/drop-done?job=%s' % job, {})
print('drop-done ->', done)

uniq = []
for s in samples:
    if not uniq or uniq[-1][1] != s[1]:
        uniq.append(s)
print('progress samples (%d distinct):' % len(uniq))
for s in uniq[:6]:
    print('   %.2fs  %d  %s' % s)
print('   ...')
for s in uniq[-3:]:
    print('   %.2fs  %d  %s' % s)

# receiver side
time.sleep(0.6)
with urllib.request.urlopen(B + '/api/state', timeout=5) as r:
    bstate = json.load(r)
recv = [t for t in bstate['transfers'] if t['dir'] == 'recv']
if recv:
    t = recv[0]
    print('receiver row: state=%s done=%d/%d' % (t['state'], t['done'], t['total']))

hits = []
for root, _, files in os.walk(r'D:\gq\test\out_b'):
    for fn in files:
        if fn == NAME:
            hits.append(os.path.join(root, fn))
print('landed files:', hits)
for p in hits:
    print('   size=%d md5=%s' % (os.path.getsize(p), md5(p)))
    print('   MATCH' if md5(p) == md5(SRC) else '   MISMATCH')
