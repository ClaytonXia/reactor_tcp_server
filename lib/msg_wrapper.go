package lib

import (
	"encoding/binary"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

func msgUnwrap(conn *Connection) (msgBytes []byte, err error) {
	if !conn.useTLV {
		tmpMsgBytes := make([]byte, 1024)
		n, err := unix.Read(conn.connFd, tmpMsgBytes)
		if err != nil {
			return nil, err
		}
		msgBytes = tmpMsgBytes[:n]
		return msgBytes, nil
	}

	var (
		lengthReadLength, contentReadLength int
	)
	if conn.LengthLeft > 0 {
		lengthBytes := make([]byte, conn.LengthLeft)
		lengthReadLength, err = unix.Read(conn.connFd, lengthBytes)
		if err != nil {
			return
		}

		connLengthBufferOffset := 4 - conn.LengthLeft
		copy(conn.LengthBuffer[connLengthBufferOffset:], lengthBytes[:lengthReadLength])
		conn.LengthLeft = conn.LengthLeft - lengthReadLength
		if lengthReadLength < conn.LengthLeft {
			log.Info("not enough length buffer")
			return
		}
		contentLength := int(binary.BigEndian.Uint32(conn.LengthBuffer))
		conn.ReadLeft = contentLength
		conn.ReadBuffer = make([]byte, contentLength)
		conn.ReadIndex = 0
	}
	contentBuffer := make([]byte, conn.ReadLeft)
	contentReadLength, err = unix.Read(conn.connFd, contentBuffer)
	if err != nil {
		return
	}
	copy(conn.ReadBuffer[conn.ReadIndex:], contentBuffer[:contentReadLength])
	readLeft := conn.ReadLeft - contentReadLength
	if readLeft == 0 {
		// dispatch message
		msgBytes = conn.ReadBuffer[:conn.ReadIndex+contentReadLength]
		conn.LengthLeft = 4
		conn.ReadBuffer = make([]byte, 1024)
		conn.ReadLeft = 0
		conn.ReadIndex = 0
		return
	}
	conn.ReadLeft = readLeft
	conn.ReadIndex += contentReadLength

	return
}

func msgWrap(msgBytes []byte, useTLV bool) []byte {
	if !useTLV {
		return msgBytes
	}

	lengthBuffer := make([]byte, 4)
	msgLength := len(msgBytes)
	dstBuffer := make([]byte, msgLength+4)
	binary.BigEndian.PutUint32(lengthBuffer, uint32(msgLength))
	copy(dstBuffer[0:4], lengthBuffer)
	copy(dstBuffer[4:], msgBytes)
	return dstBuffer
}
