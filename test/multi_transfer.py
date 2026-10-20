import threading, urllib.request, urllib.error, json, time, os, hashlib, glob

BASE = 'http://127.0.0.1:45800'
SRC = r'D:\gq\test\测试文件_8MB.bin'
OUT = r'D:\gq\test\out_a'
N = 3

results = {}
sessions = {}
lock = threading.Lock()


def post_json(path, obj, timeout=20):
    body = json.dumps(obj).encode()
    req = urllib.request.Request(BASE + path, data=body,
                                 headers={'Content-Type': 'application/json'}, method='POST')
    with urllib.request.urlopen(req, timeout=timeout) as r:
        return r.status, r.read().decode()


def state():
    with urllib.request.urlopen(BASE + '/api/state', timeout=3) as r:
        return json.load(r)


def sender(i):
    name = '用户%d' % i
    try:
        st, body = post_json('/api/offer', {
            "from": {"id": "u%d" % i, "name": name},
            "files": [{"name": 'job%d.bin' % i, "size": os.path.getsize(SRC)}],
        }, timeout=30)
        sid = json.loads(body)['session']
        with lock:
            sessions[i] = sid

        size = os.path.getsize(SRC)
        with open(SRC, 'rb') as f:
            data = f.read()
        req = urllib.request.Request(
            BASE + '/api/upload?session=%s&idx=0' % sid, data=data,
            headers={'Content-Type': 'application/octet-stream'}, method='POST')
        with urllib.request.urlopen(req, timeout=60) as r:
            up = r.status
        post_json('/api/done?session=%s' % sid, {})
        results[i] = ('ok', up, size)
    except Exception as e:
        results[i] = ('ERR', str(e))


ts = [threading.Thread(target=sender, args=(i,)) for i in range(N)]
for t in ts:
    t.start()
time.sleep(1.5)

s = state()
print('排队数:', len(s['offers']), [o['sender'] for o in s['offers']])

for _ in range(N):
    s = state()
    if not s['offers']:
        break
    head = s['offers'][0]
    st, _ = post_json('/api/accept', {"id": head['id']})
    print('  接受', head['sender'], st)
    time.sleep(0.4)

for t in ts:
    t.join(timeout=60)

print('发送结果:')
for i in range(N):
    print('  用户%d:' % i, results.get(i))

print('接收端状态:')
s = state()
for t in s['transfers'][:6]:
    print('  ', t['peer'], t['state'], t['done'], '/', t['total'])

print('落盘文件:')
src_md5 = hashlib.md5(open(SRC, 'rb').read()).hexdigest()
for p in sorted(glob.glob(os.path.join(OUT, '*', 'job*.bin')))[-6:]:
    h = hashlib.md5(open(p, 'rb').read()).hexdigest()
    print('  ', os.path.relpath(p, OUT), os.path.getsize(p), 'md5', 'OK' if h == src_md5 else 'MISMATCH')
