package reactor_tcp_server

func wrapErrorCallback(server *Server, conn *Connection, err error, errFunc func(*Server, *Connection, error)) func() {
	return func() {
		errFunc(server, conn, err)
	}
}

func wrapMessageCallback(server *Server, conn *Connection, msg []byte, msgFunc func(*Server, *Connection, []byte)) func() {
	return func() {
		msgFunc(server, conn, msg)
	}
}

func wrapGeneralCallback(server *Server, conn *Connection, generalFunc func(*Server, *Connection)) func() {
	return func() {
		generalFunc(server, conn)
	}
}
