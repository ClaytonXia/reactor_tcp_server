package lib

import (
	ants "github.com/panjf2000/ants/v2"
	log "github.com/sirupsen/logrus"
)

var (
	pool         *ants.Pool
	taskQueueCap = 100000
	taskChan     = make(chan *AsyncTask, taskQueueCap)
)

const (
	TaskMessage = iota + 1
	TaskError
	TaskGeneral
)

type AsyncTask struct {
	TaskFunc func()
	TaskType int
	Server   *Server
	Conn     *Connection
}

func NewPool(workerNum int) {
	if workerNum <= 0 {
		workerNum = 100
	}
	tmpPool, err := ants.NewPool(workerNum, ants.WithMaxBlockingTasks(100))
	if err != nil {
		panic(err)
	}
	pool = tmpPool
}

func MessageTaskEnqueue(server *Server, conn *Connection, msg []byte, msgFunc func(server *Server, conn *Connection, msg []byte)) {
	taskChan <- &AsyncTask{
		TaskType: TaskMessage,
		TaskFunc: wrapMessageCallback(server, conn, msg, msgFunc),
		Server:   server,
		Conn:     conn,
	}
}

func GeneralTaskEnqueue(server *Server, conn *Connection, generalFunc func(server *Server, conn *Connection)) {
	taskChan <- &AsyncTask{
		TaskType: TaskGeneral,
		TaskFunc: wrapGeneralCallback(server, conn, generalFunc),
		Server:   server,
		Conn:     conn,
	}
}

func ErrorTaskEnqueue(server *Server, conn *Connection, err error, errFunc func(server *Server, conn *Connection, err error)) {
	taskChan <- &AsyncTask{
		TaskType: TaskError,
		TaskFunc: wrapErrorCallback(server, conn, err, errFunc),
		Server:   server,
		Conn:     conn,
	}
}

func TaskConsume() {
	for asyncTask := range taskChan {
		taskType, taskFunc, server, conn := asyncTask.TaskType, asyncTask.TaskFunc, asyncTask.Server, asyncTask.Conn
		if conn.IsClosed() {
			continue
		}
		err := submitTask(taskFunc)
		if err != nil {
			if taskType != TaskError {
				conn.onErrorCallback(server, conn, err)
				continue
			}
			log.Printf("task exec err: %v", err)
			continue
		}
		log.Infof("task submitted: %d", taskType)
		// close connection
		if taskType == TaskError && !conn.IsClosed() {
			conn.Close()
		}
	}
}

func submitTask(taskFunc func()) (err error) {
	err = pool.Submit(taskFunc)
	if err != nil {
		log.Errorf("submit task err: %v", err)
		return
	}

	return
}
