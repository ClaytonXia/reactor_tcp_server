package lib

import (
	filename "github.com/keepeye/logrus-filename"
	log "github.com/sirupsen/logrus"
)

func InitLog() {
	log.SetFormatter(&log.JSONFormatter{})
	filenameHook := filename.NewHook()
	filenameHook.Field = "line"
	log.AddHook(filenameHook)
}
