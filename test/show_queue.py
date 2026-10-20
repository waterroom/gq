"""Hold three inbound offers open so the queue can be screenshotted."""
import threading, urllib.request, json, time

BASE = 'http://127.0.0.1:45800'


def offer(i):
    body = json.dumps({
        "from": {"id": "u%d" % i, "name": ["张伟", "李娜", "王强"][i - 1]},
        "files": [{"name": "设计稿_v%d.zip" % i, "size": 24 * 1024 * 1024},
                  {"name": "readme.txt", "size": 1024}],
    }).encode()
    req = urllib.request.Request(BASE + '/api/offer', data=body,
                                 headers={'Content-Type': 'application/json'}, method='POST')
    try:
        with urllib.request.urlopen(req, timeout=60) as r:
            r.read()
    except Exception:
        pass


for i in (1, 2, 3):
    threading.Thread(target=offer, args=(i,), daemon=True).start()
    time.sleep(0.15)

time.sleep(25)
