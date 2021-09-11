package reactor_tcp_server

import "testing"

func TestServer(t *testing.T) {
	host := "0.0.0.0"
	port := 9999
	server := NewServer(host, port, 0)
	server.SetWorkerNum(100)
	server.Run()
}
