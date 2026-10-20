import socket, struct, time, json, urllib.request

MCAST='239.255.72.89'; PORT=45800
SRC='192.168.0.172'   # 强制从这个网卡发

s=socket.socket(socket.AF_INET, socket.SOCK_DGRAM, socket.IPPROTO_UDP)
s.setsockopt(socket.IPPROTO_IP, socket.IP_MULTICAST_IF, socket.inet_aton(SRC))
s.setsockopt(socket.IPPROTO_IP, socket.IP_MULTICAST_TTL, 2)
msg=json.dumps({"t":"hi","id":"testfromA0001","name":"TEST-FROM-192.168.0.172","port":45800,"os":"windows"}).encode()
for _ in range(4):
    s.sendto(msg,(MCAST,PORT)); time.sleep(0.4)
print('已从', SRC, '发出 4 个 announce')

time.sleep(0.8)
with urllib.request.urlopen('http://192.168.0.171:45800/api/state', timeout=4) as r:
    d=json.load(r)
print('B 看到的设备:', [(p['name'],p['addr']) for p in d['peers']])
