import socket, struct, time
MCAST='239.255.72.89'; PORT=45800
s=socket.socket(socket.AF_INET, socket.SOCK_DGRAM, socket.IPPROTO_UDP)
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
try:
    s.bind(('', PORT))
except OSError as e:
    print('bind fail', e)
mreq = struct.pack('4s4s', socket.inet_aton(MCAST), socket.inet_aton('0.0.0.0'))
s.setsockopt(socket.IPPROTO_IP, socket.IP_ADD_MEMBERSHIP, mreq)
s.settimeout(1.0)
print('listening 4s ...')
t0=time.time()
got=0
while time.time()-t0 < 4:
    try:
        d,a=s.recvfrom(2048)
        print('recv from', a, d[:120])
        got+=1
    except socket.timeout:
        pass
print('total', got)
