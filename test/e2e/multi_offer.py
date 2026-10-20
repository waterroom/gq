import threading, urllib.request, urllib.error, json, time

BASE = 'http://127.0.0.1:45800'
results = {}


def offer(i):
    body = json.dumps({
        "from": {"id": "u%d" % i, "name": "用户%d" % i},
        "files": [{"name": "f%d.txt" % i, "size": 10}],
    }).encode()
    req = urllib.request.Request(BASE + '/api/offer', data=body,
                                 headers={'Content-Type': 'application/json'}, method='POST')
    try:
        with urllib.request.urlopen(req, timeout=15) as r:
            results[i] = (r.status, r.read().decode())
    except urllib.error.HTTPError as e:
        results[i] = (e.code, e.read().decode())
    except Exception as e:
        results[i] = ('ERR', str(e))


def state():
    with urllib.request.urlopen(BASE + '/api/state', timeout=3) as r:
        return json.load(r)


ts = [threading.Thread(target=offer, args=(i,)) for i in (1, 2, 3)]
for t in ts:
    t.start()
time.sleep(1.5)

s = state()
print('并发到达后排队数:', len(s['offers']), [o['sender'] for o in s['offers']])

for _ in range(3):
    s = state()
    if not s['offers']:
        break
    head = s['offers'][0]
    body = json.dumps({"id": head['id']}).encode()
    req = urllib.request.Request(BASE + '/api/accept', data=body,
                                 headers={'Content-Type': 'application/json'}, method='POST')
    with urllib.request.urlopen(req, timeout=3) as r:
        print('  接受', head['sender'], '->', r.status)
    time.sleep(0.6)

for t in ts:
    t.join(timeout=10)

print('三个发送方最终结果:')
for i in (1, 2, 3):
    print('  用户%d:' % i, results.get(i))
