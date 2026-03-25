package logger

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

type Options struct {
	RoomID string
	UserID string
}

// 供外部传入的工作目录
var WorkDir string

func Init(wd string) {
	WorkDir = wd
}

func writeLogToFile(path string, data []byte) bool {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("Failed to open log file %s: %v\n", path, err)
		return false
	}
	defer file.Close()

	if _, err := file.Write(data); err != nil {
		log.Printf("Failed to write to log file %s: %v\n", path, err)
		return false
	}
	return true
}

func Log(tp string, info string, opt Options) (ok bool) {
	now := time.Now().Format("2006-01-02 15:04:05")

	switch tp {
	case "error":
		path := filepath.Join(WorkDir, "logs", "error.log")
		byteData := []byte(fmt.Sprintf("[ERROR|%s]:%s\n", now, info))
		return writeLogToFile(path, byteData)

	case "info":
		path := filepath.Join(WorkDir, "logs", "info.log")
		byteData := []byte(fmt.Sprintf("[INFO|%s]:%s\n", now, info))
		return writeLogToFile(path, byteData)

	case "bot":
		path := filepath.Join(WorkDir, "logs", "bot.log")
		byteData := []byte(fmt.Sprintf("[%s|%s|%s]:%s\n", opt.UserID, opt.RoomID, now, info))
		return writeLogToFile(path, byteData)

	default:
		return false
	}
}
