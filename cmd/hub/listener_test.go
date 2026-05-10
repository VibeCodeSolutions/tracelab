package main

import (
	"net"
)

// listenerInfo is a tiny holder for newListener's return value so
// main_test.go can stay framework-free (no test-utility import).
type listenerInfo struct {
	net.Listener
	port int
}

// newListener binds 127.0.0.1:0 and returns the listener + chosen port.
// Caller is responsible for Close. Used to find a free TCP port for
// the integration test without racing on a fixed port number.
func newListener() (*listenerInfo, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	return &listenerInfo{Listener: ln, port: ln.Addr().(*net.TCPAddr).Port}, nil
}
