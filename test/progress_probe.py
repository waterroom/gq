"""Verify the receiver's progress advances continuously, not in one jump."""
import http.client
import json
import threading
import time
import urllib.request
import os

HOST, PORT = '127.0.0.1', 45800
SRC = r'D:\gq\test\测试文件_8MB.bin'
CHUNK = 64 * 1024
DELAY = 0.008

samples = []
stop = threading.Event()


def poll():
    while not stop.is_set():
        try:
            with urllib.request.urlopen('http://%s:%d/api/state' % (HOST, PORT), timeout=2) as r:
                d = json.load(r)
            if d['transfers']:
                t = d['transfers'][0]
                samples.append((round(time.time() - t0, 2), t['done'], t['state'], t['speed']))
        except Exception:
            pass
        time.sleep(0.1)


# 1. negotiate
body = json.dumps({
    "from": {"id": "probe0001", "name": "进度探针"},
    "files": [{"name": "progress.bin", "size": os.path.getsize(SRC)}],
}).encode()
req = urllib.request.Request('http://%s:%d/api/offer' % (HOST, PORT), data=body,
                             headers={'Content-Type': 'application/json'}, method='POST')
with urllib.request.urlopen(req, timeout=10) as r:
    sid = json.load(r)['session']

t0 = time.time()
th = threading.Thread(target=poll, daemon=True)
th.start()

# 2. stream the body in chunks so the server sees data arriving progressively
size = os.path.getsize(SRC)
conn = http.client.HTTPConnection(HOST, PORT, timeout=60)
conn.putrequest('POST', '/api/upload?session=%s&idx=0' % sid)
conn.putheader('Content-Type', 'application/octet-stream')
conn.putheader('Content-Length', str(size))
conn.endheaders()
with open(SRC, 'rb') as f:
    while True:
        chunk = f.read(CHUNK)
        if not chunk:
            break
        conn.send(chunk)
        time.sleep(DELAY)
resp = conn.getresponse()
print('upload status:', resp.status)
conn.close()

time.sleep(0.5)
stop.set()
th.join(timeout=2)

print('采样（时间, 已收字节, 状态, 速度）:')
prev = -1
increasing = 0
for s in samples:
    print('  ', s)
    if s[1] > prev:
        increasing += 1
    prev = s[1]
print('进度递增的采样点: %d / %d' % (increasing, len(samples)))
