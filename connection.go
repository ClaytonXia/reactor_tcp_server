package reactor_tcp_server

import (
	"os"
	"sync/atomic"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

type Connection struct {
	server            *Server
	connFile          *os.File
	connFd            int
	Epoller           *Epoller
	onMessageCallback func(*Server, *Connection, []byte)
	onErrorCallback   func(*Server, *Connection, error)
	Event             int
	ReadBuffer        []byte
	ReadLeft          int
	ReadIndex         int
	WriteBuffer       []byte
	LengthBuffer      []byte
	LengthLeft        int
	isClosed          int32
	useTLV            bool
}

func NewConnection(Server *Server, connFile *os.File, epoller *Epoller) *Connection {
	return &Connection{
		server:            Server,
		connFile:          connFile,
		connFd:            int(connFile.Fd()),
		Epoller:           epoller,
		onMessageCallback: Server.onMessageCallback,
		onErrorCallback:   Server.onErrorCallback,
		LengthLeft:        4,
		LengthBuffer:      make([]byte, 4),
		useTLV:            Server.useTLV,
	}
}

func (conn *Connection) ProcessConnectionEvent(events int) {
	switch {
	case events&unix.EPOLLRDHUP == unix.EPOLLRDHUP:
		conn.Close()
	case events&unix.EPOLLIN == unix.EPOLLIN:
		conn.onReadable()
	case events&unix.EPOLLOUT == unix.EPOLLOUT:
		conn.onWritable()
	default:
		log.Warnf("unprocessed event: %d", events)
	}
}

func (conn *Connection) onReadable() {
	for {
		msgBytes, err := msgUnwrap(conn)
		if err == unix.EAGAIN {
			break
		}
		if err != nil {
			log.Errorf("message parse err: %v", err)
			defer conn.Close()
			conn.onErrorCallback(conn.server, conn, err)
			return
		}
		if msgBytes != nil {
			conn.onMessageCallback(conn.server, conn, msgBytes)
			continue
		}
	}
}

func (conn *Connection) onWritable() {
	writeBufferLength := len(conn.WriteBuffer)
	if writeBufferLength == 0 {
		return
	}
	writeLen, err := unix.Write(conn.connFd, conn.WriteBuffer)
	if err != nil {
		conn.onErrorCallback(conn.server, conn, err)
		return
	}
	if writeBufferLength == writeLen {
		conn.Epoller.Edit(conn, ReadEvent)
	}
	conn.WriteBuffer = conn.WriteBuffer[writeLen:]
}

func (conn *Connection) Write(msg []byte) {
	tlvMsgBytes := msgWrap(msg, conn.useTLV)
	conn.WriteBuffer = append(conn.WriteBuffer, tlvMsgBytes...)
	conn.Epoller.Edit(conn, ReadWriteEvent)
}

func (conn *Connection) Close() {
	conn.connFile.Close()
	_ = conn.Epoller.Remove(conn)
	_ = unix.Close(conn.connFd)
	atomic.StoreInt32(&conn.isClosed, 1)
}

func (conn *Connection) IsClosed() bool {
	return atomic.LoadInt32(&conn.isClosed) == 1
}

func (conn *Connection) GetErrorCallback() func(*Server, *Connection, error) {
	return conn.onErrorCallback
}

func (conn *Connection) task(taskFunc func(*Server, *Connection)) {
	GeneralTaskEnqueue(conn.server, conn, taskFunc)
}
