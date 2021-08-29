package lib

import (
	"sync"
	"syscall"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

var (
	ReadEvent              = unix.EPOLLIN | unix.EPOLLRDHUP | unix.EPOLLET
	ReadWriteEvent         = ReadEvent | unix.EPOLLOUT
	wakeUpMessage          = []byte{0, 0, 0, 0, 0, 0, 0, 1}
	defaultEventWaitLength = 1024.0
)

type Epoller struct {
	fd          int
	wakeUpFd    int
	connections map[int]*Connection
	lock        *sync.RWMutex
}

type ConnEvent struct {
	Fd     int
	Conn   *Connection
	Events int
}

func NewEpoller() (*Epoller, error) {
	fd, err := unix.EpollCreate1(0)
	if err != nil {
		return nil, err
	}

	r0, _, errno := unix.Syscall(unix.SYS_EVENTFD2, 0, 0, 0)
	if errno != 0 {
		_ = unix.Close(fd)
		return nil, errno
	}
	err = unix.EpollCtl(fd, unix.EPOLL_CTL_ADD, int(r0), &unix.EpollEvent{
		Events: uint32(ReadEvent),
		Fd:     int32(r0),
	})
	if err != nil {
		_ = unix.Close(fd)
		_ = unix.Close(int(r0))
		return nil, err
	}

	return &Epoller{
		fd:          fd,
		wakeUpFd:    int(r0),
		lock:        &sync.RWMutex{},
		connections: make(map[int]*Connection),
	}, nil
}

func (e *Epoller) Add(conn *Connection) error {
	err := syscall.SetNonblock(conn.connFd, true)
	if err != nil {
		log.Errorf("set conn non block err: %v", err)
		conn.Close()
		return err
	}

	err = unix.EpollCtl(e.fd, syscall.EPOLL_CTL_ADD, conn.connFd, &unix.EpollEvent{Events: uint32(ReadEvent), Fd: int32(conn.connFd)})
	if err != nil {
		return err
	}

	e.lock.Lock()
	defer e.lock.Unlock()
	e.connections[conn.connFd] = conn

	return nil
}

func (e *Epoller) Edit(conn *Connection, event int) error {
	err := unix.EpollCtl(e.fd, syscall.EPOLL_CTL_MOD, conn.connFd, &unix.EpollEvent{Events: uint32(event), Fd: int32(conn.connFd)})
	if err != nil {
		return err
	}

	return nil
}

func (e *Epoller) Remove(conn *Connection) error {
	err := unix.EpollCtl(e.fd, syscall.EPOLL_CTL_DEL, conn.connFd, nil)
	if err != nil {
		return errors.WithMessage(err, "epoll ctrl err")
	}

	e.lock.Lock()
	defer e.lock.Unlock()
	delete(e.connections, conn.connFd)
	return nil
}

func (e *Epoller) Wait() ([]*ConnEvent, error) {
	for {
		events := make([]unix.EpollEvent, int(defaultEventWaitLength))
		n, err := unix.EpollWait(e.fd, events, 3000)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			return nil, err
		}
		e.lock.RLock()
		defer e.lock.RUnlock()
		var connEvents []*ConnEvent
		for i := 0; i < n; i++ {
			if events[i].Fd == int32(e.wakeUpFd) {
				connEvents = append(connEvents, &ConnEvent{Fd: e.wakeUpFd})
				continue
			}
			conn := e.connections[int(events[i].Fd)]
			connEvents = append(connEvents, &ConnEvent{Conn: conn, Events: int(events[i].Events)})
		}
		if n >= int(0.75*defaultEventWaitLength) {
			defaultEventWaitLength = defaultEventWaitLength * 2
		}
		return connEvents, nil
	}
}

func (e *Epoller) wakeUp() (err error) {
	_, err = unix.Write(e.wakeUpFd, wakeUpMessage)
	if err != nil {
		log.Errorf("wake err: %v", err)
	}
	return
}

func (e *Epoller) wakeUpRead() (err error) {
	wakeUpBytes := make([]byte, 8)
	_, err = unix.Read(e.wakeUpFd, wakeUpBytes)
	if err != nil {
		log.Errorf("waking up read err: %v", err)
	}
	return
}

func (e *Epoller) process() {
	for {
		connEvents, err := e.Wait()
		if err != nil {
			log.Errorf("epoller wait err: %v", err)
			continue
		}
		for _, connEvent := range connEvents {
			if connEvent.Fd == e.wakeUpFd {
				e.wakeUpRead()
				continue
			}
			connEvent.Conn.ProcessConnectionEvent(connEvent.Events)
		}
	}
}
