//go:build !windows

package app

import "net"

func listenMulticast(port int) (*net.UDPConn, error) {
	addr := &net.UDPAddr{IP: net.ParseIP(MulticastIP), Port: port}
	return net.ListenMulticastUDP("udp4", nil, addr)
}
