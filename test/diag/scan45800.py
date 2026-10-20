import socket, concurrent.futures, ipaddress

SUBNETS = ['192.168.0', '192.168.1', '192.168.90', '192.168.100', '192.168.134']
PORT = 45800

def probe(ip):
    s = socket.socket()
    s.settimeout(0.25)
    try:
        if s.connect_ex((str(ip), PORT)) == 0:
            return str(ip)
    finally:
        s.close()
    return None

targets = []
for net in SUBNETS:
    for h in range(1, 255):
        targets.append(ipaddress.ip_address('%s.%d' % (net, h)))

hits = []
with concurrent.futures.ThreadPoolExecutor(max_workers=256) as ex:
    for r in ex.map(probe, targets):
        if r:
            hits.append(r)

print('扫描 %d 个地址，开放 %d 端口:' % (len(targets), PORT))
for h in sorted(hits):
    print('  ', h)
