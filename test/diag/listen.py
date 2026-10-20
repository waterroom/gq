import socket, struct, time, json
MCAST='239.255.72.89'; PORT=45800
s=socket.socket(socket.AF_INET, socket.SOCK_DGRAM, socket.IPPROTO_UDP)
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind(('', PORT))
for ip in ['0.0.0.0']:
    mreq = struct.pack('4s4s', socket.inet_aton(MCAST), socket.inet_aton(ip))
    try:
        s.setsockopt(socket.IPPROTO_IP, socket.IP_ADD_MEMBERSHIP, mreq)
    except OSError as e:
        print('join fail', e)
s.settimeout(1.0)
seen={}
t0=time.time()
while time.time()-t0 < 10:
    try:
        d,a=s.recvfrom(2048)
        try: m=json.loads(d)
        except Exception: continue
        key=(m.get('id'),m.get('name'),a[0])
        if key not in seen:
            seen[key]=time.time()
            print('收到: id=%s name=%s from %s port=%s' % (m.get('id'), m.get('name'), a[0], m.get('port')), flush=True)
    except socket.timeout:
        pass
print('总计不同来源:', len(seen))
