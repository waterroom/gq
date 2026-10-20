"""Receive-side check: files land directly in the receive folder, and two
uploads carrying the same filename end up as "name" and "name (1)" - including
when they are in flight at the same moment from two different peers.

usage: python recv_flat.py <senderA_port> <senderC_port> <recvB_port>
"""
import json
import os
import sys
import threading
import time
import urllib.request

A = int(sys.argv[1]) if len(sys.argv) > 1 else 45800
C = int(sys.argv[2]) if len(sys.argv) > 2 else 46000
B = int(sys.argv[3]) if len(sys.argv) > 3 else 45900

SRC = r"D:\gq\test\src"
OUT = r"D:\gq\test\out_b"
SENDER_A = "aaaaaaaaaaaaaaaa"
SENDER_C = "cccccccccccccccc"


def post(port, path, body, timeout=20):
    req = urllib.request.Request(
        f"http://127.0.0.1:{port}{path}",
        data=json.dumps(body).encode(),
        headers={"Content-Type": "application/json"},
    )
    with urllib.request.urlopen(req, timeout=timeout) as r:
        return r.status, json.loads(r.read() or b"{}")


def state(port):
    with urllib.request.urlopen(f"http://127.0.0.1:{port}/api/state", timeout=5) as r:
        return json.load(r)


def wait_idle(port, seconds=40):
    t0 = time.time()
    while time.time() - t0 < seconds:
        ss = state(port)["transfers"]
        if all(x["state"] in ("done", "error", "canceled") for x in ss):
            return ss
        time.sleep(0.2)
    return state(port)["transfers"]


os.makedirs(SRC, exist_ok=True)
os.makedirs(OUT, exist_ok=True)

# Two distinct payloads that share a filename: sizes must differ so the result
# proves which bytes landed where.
seq = os.path.join(SRC, "重名.bin")
seq2 = os.path.join(SRC, "重名2.bin")
conc_a = os.path.join(SRC, "并发同名_a.bin")
conc_c = os.path.join(SRC, "并发同名_c.bin")

open(seq, "wb").write(b"A" * (96 * 1024))
open(seq2, "wb").write(b"Z" * (96 * 1024))
open(conc_a, "wb").write(b"1" * (4 * 1024 * 1024))
open(conc_c, "wb").write(b"2" * (6 * 1024 * 1024))

print("== 1. A 连发两次同一个文件名 ==")
for i in (1, 2):
    st, body = post(A, "/api/send-path", {"peer": "bbbbbbbbbbbbbbbb", "paths": [seq]})
    print(f"   第{i}次 -> {st} {body}")
    wait_idle(A)
time.sleep(0.5)
print("   out_b:", sorted(os.listdir(OUT)))

print("== 2. A 与 C 同时发同名文件 ==")
results = {}


def fire(port, peer, path, tag):
    try:
        results[tag] = post(port, "/api/send-path", {"peer": peer, "paths": [path]})
    except Exception as e:  # noqa: BLE001
        results[tag] = repr(e)


# File contents differ per sender, so give them the same *name* on disk by
# copying into a per-sender dir first.
os.makedirs(os.path.join(SRC, "a"), exist_ok=True)
os.makedirs(os.path.join(SRC, "c"), exist_ok=True)
pa = os.path.join(SRC, "a", "并发同名.bin")
pc = os.path.join(SRC, "c", "并发同名.bin")
open(pa, "wb").write(b"1" * (4 * 1024 * 1024))
open(pc, "wb").write(b"2" * (6 * 1024 * 1024))

ta = threading.Thread(target=fire, args=(A, "bbbbbbbbbbbbbbbb", pa, "A"))
tc = threading.Thread(target=fire, args=(C, "bbbbbbbbbbbbbbbb", pc, "C"))
ta.start()
tc.start()
ta.join()
tc.join()
print("   请求结果:", results)
wait_idle(A)
wait_idle(C)
time.sleep(0.5)

print("== 3. 落盘结果 ==")
names = sorted(os.listdir(OUT))
for n in names:
    p = os.path.join(OUT, n)
    if os.path.isfile(p):
        with open(p, "rb") as f:
            head = f.read(1)
        print(f"   {n:28s} {os.path.getsize(p):>9d} B  首字节={head!r}")
print("   子目录:", [d for d in names if os.path.isdir(os.path.join(OUT, d))] or "无")
