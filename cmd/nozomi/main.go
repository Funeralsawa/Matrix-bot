package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"nozomi/internal/app"
	"nozomi/internal/config"
)

func main() {
	// 1. 加载所有配置文件
	cfg := config.NewBotConfig()

	// 2. 初始化整个应用
	nozomiApp, err := app.NewApp(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize app: %v", err)
	}

	// 3. 创建带有取消功能的上下文，用于优雅退出
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 4. 在独立协程中启动应用
	go func() {
		if err := nozomiApp.Start(ctx); err != nil {
			log.Fatalf("App stopped with error: %v", err)
		}
	}()

	// 5. 监听 Linux 系统的 Ctrl+C (SIGINT) 或 kill (SIGTERM) 信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Received shutdown signal. Shutting down...")
	// 触发上下文取消，通知所有后台定时任务停止
	cancel()
	// 关闭数据库和 Matrix 同步
	nozomiApp.Stop()
	log.Println("Nozomi exited successfully.")
}
