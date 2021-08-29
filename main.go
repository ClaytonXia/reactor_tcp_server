package main

import (
	"github.com/claytonxia/reactor_tcp_server/lib"
)

func main() {
	host := "0.0.0.0"
	port := 9999
	server := lib.NewServer(host, port, 0)
	server.SetWorkerNum(100)
	server.Run()
}
