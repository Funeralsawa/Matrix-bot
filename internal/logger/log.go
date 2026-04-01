package logger

import (
	"fmt"
	"log"
	"nozomi/internal/config"
	"os"
	"path/filepath"
	"time"
)

type Options struct {
	RoomID string
	UserID string
}

type Logger struct {
	workDir string
}

func (l *Logger) Init(wd string) {
	l.workDir = wd
}

func (l *Logger) writeLogToFile(path string, data []byte) bool {
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

func (l *Logger) Log(tp string, info string, opt Options) (ok bool) {
	now := time.Now().Format("2006-01-02 15:04:05")

	switch tp {
	case "error":
		path := filepath.Join(l.workDir, "logs", "error.log")
		byteData := []byte(fmt.Sprintf("[ERROR|%s]:%s\n", now, info))
		return l.writeLogToFile(path, byteData)

	case "info":
		path := filepath.Join(l.workDir, "logs", "info.log")
		byteData := []byte(fmt.Sprintf("[INFO|%s]:%s\n", now, info))
		return l.writeLogToFile(path, byteData)

	case "bot":
		path := filepath.Join(l.workDir, "logs", "bot.log")
		byteData := []byte(fmt.Sprintf("[%s|%s|%s]:%s\n", opt.UserID, opt.RoomID, now, info))
		return l.writeLogToFile(path, byteData)

	default:
		return false
	}
}

func NewLogger(cfg *config.BotConfig) *Logger {
	return &Logger{workDir: cfg.WorkDir}
}
