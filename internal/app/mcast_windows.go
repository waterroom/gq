//go:build windows

package app

import (
	"net"
	"os"
	"syscall"
)

// listenMulticast creates the discovery socket by hand.
//
// net.ListenMulticastUDP binds the socket to the *multicast address*, which the
// Windows stack accepts but then never delivers group traffic to. Binding
// INADDR_ANY and joining afterwards is what actually works here.
func listenMulticast(port int) (*net.UDPConn, error) {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, syscall.IPPROTO_UDP)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), "mcast")
	defer f.Close()

	if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
		return nil, err
	}
	// SO_REUSEADDR must precede bind; SockaddrInet4 with a zero Addr is INADDR_ANY.
	if err := syscall.Bind(fd, &syscall.SockaddrInet4{Port: port}); err != nil {
		return nil, err
	}
	_ = syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_RCVBUF, 256*1024)

	group := [4]byte{239, 255, 72, 89}
	joined := 0
	for _, ip := range localIPv4s() {
		v4 := ip.To4()
		if v4 == nil {
			continue
		}
		mreq := &syscall.IPMreq{Multiaddr: group}
		copy(mreq.Interface[:], v4)
		if err := syscall.SetsockoptIPMreq(fd, syscall.IPPROTO_IP, syscall.IP_ADD_MEMBERSHIP, mreq); err == nil {
			joined++
		}
	}
	if joined == 0 {
		mreq := &syscall.IPMreq{Multiaddr: group}
		if err := syscall.SetsockoptIPMreq(fd, syscall.IPPROTO_IP, syscall.IP_ADD_MEMBERSHIP, mreq); err != nil {
			return nil, err
		}
	}

	pc, err := net.FilePacketConn(f)
	if err != nil {
		return nil, err
	}
	conn, ok := pc.(*net.UDPConn)
	if !ok {
		pc.Close()
		return nil, syscall.EINVAL
	}
	return conn, nil
}
