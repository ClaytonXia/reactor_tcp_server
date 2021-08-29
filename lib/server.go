package lib

import (
	"fmt"
	"net"
	"runtime"
	"syscall"

	reuseport "github.com/libp2p/go-reuseport"
	log "github.com/sirupsen/logrus"
)

// Server _
type Server struct {
	onStartCallback   func(*Server)
	onConnectCallback func(*Server, *Connection)
	onMessageCallback func(*Server, *Connection, []byte)
	onErrorCallback   func(*Server, *Connection, error)
	server            *net.TCPListener
	subReactorNum     int
	subReactors       []*Epoller
	workerNum         int
	useTLV            bool
	nextSubReactor    int
}

var (
	defaultConnectCallback = func(server *Server, conn *Connection) {
		log.Infof("new client connected")
	}
	defaultStartCallback = func(server *Server) {
		log.Info("server started")
	}
	defaultMessageCallback = func(server *Server, conn *Connection, msg []byte) {
		log.Infof("new client message: %s", string(msg))
		conn.Write(msg)
	}
	defaultErrorCallback = func(server *Server, conn *Connection, err error) {
		log.Errorf("new error: %v", err)
	}
)

func NewServer(host string, port int, subReactorNum int) *Server {
	InitLog()

	listener, err := reuseport.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		panic(err)
	}
	tcpServer, _ := listener.(*net.TCPListener)
	serverFile, err := tcpServer.File()
	if err != nil {
		panic(err)
	}
	serverFd := int(serverFile.Fd())
	err = syscall.SetNonblock(serverFd, true)
	if err != nil {
		panic(err)
	}

	if subReactorNum <= 0 {
		subReactorNum = runtime.NumCPU()
	}
	subReactors, err := initSubReactors(subReactorNum)
	if err != nil {
		panic(err)
	}

	return &Server{
		server:            tcpServer,
		onStartCallback:   defaultStartCallback,
		onConnectCallback: defaultConnectCallback,
		onMessageCallback: defaultMessageCallback,
		onErrorCallback:   defaultErrorCallback,
		subReactorNum:     subReactorNum,
		subReactors:       subReactors,
	}
}

func initSubReactors(subReactorNum int) (subReactors []*Epoller, err error) {
	for i := 0; i < subReactorNum; i++ {
		tmpSubReactor, err := NewEpoller()
		if err != nil {
			return nil, err
		}
		subReactors = append(subReactors, tmpSubReactor)
	}

	return
}

func (Server *Server) OnConnect(connFunc func(*Server, *Connection)) {
	Server.onConnectCallback = connFunc
}

func (Server *Server) OnMessage(msgFunc func(*Server, *Connection, []byte)) {
	Server.onMessageCallback = msgFunc
}

func (Server *Server) onError(errFunc func(*Server, *Connection, error)) {
	Server.onErrorCallback = errFunc
}

func (Server *Server) setUseTLV() {
	Server.useTLV = true
}

func (Server *Server) Run() {
	NewPool(Server.workerNum)

	// worker pool
	go TaskConsume()

	// main io event loop responsible for connection accept
	epoller, err := NewEpoller()
	if err != nil {
		panic(err)
	}
	go epoller.process()

	// sub io event loop responsible for io event dispatch
	for _, subReactor := range Server.subReactors {
		go subReactor.process()
	}

	// on server start
	Server.onStartCallback(Server)

	// client accept
	for {
		conn, err := Server.server.AcceptTCP()
		if err != nil {
			log.Errorf("accept err: %v", err)
			continue
		}
		connFile, err := conn.File()
		if err != nil {
			log.Errorf("conn file err: %v", err)
			continue
		}
		connEpoller := Server.selectEpoller()
		xConn := NewConnection(Server, connFile, connEpoller)
		err = xConn.Epoller.Add(xConn)
		if err != nil {
			log.Errorf("add conn to event loop err: %v", err)
			conn.Close()
			continue
		}
		xConn.Epoller.wakeUp()

		// onConnectCallback
		Server.onConnectCallback(Server, xConn)
	}
}

func (Server *Server) selectEpoller() *Epoller {
	epoller := Server.subReactors[Server.nextSubReactor]
	Server.nextSubReactor = (Server.nextSubReactor + 1) % Server.subReactorNum

	return epoller
}

func (Server *Server) SetWorkerNum(workerNum int) {
	Server.workerNum = workerNum
}
