package handler

import (
	"sync"
	"time"

	"nozomi/internal/billing"
	"nozomi/internal/config"
	"nozomi/internal/llm"
	"nozomi/internal/logger"
	"nozomi/internal/matrix"
	"nozomi/internal/memory"
	"nozomi/internal/quota"
	"nozomi/internal/ratelimit"
)

type Router struct {
	matrix           *matrix.Client
	llm              *llm.Client
	memory           *memory.Manager
	billing          *billing.System
	cfg              *config.BotConfig
	logger           *logger.Logger
	quota            *quota.Manager
	rateManager      *ratelimit.RateManager
	bootTime         time.Time // 用于过滤启动前的历史陈旧消息
	pendingApprovals sync.Map  // map[tools.Task]chan bool
}

func NewRouter(m *matrix.Client, l *llm.Client, mem *memory.Manager, b *billing.System, cfg *config.BotConfig, logger *logger.Logger, quota *quota.Manager, rateManager *ratelimit.RateManager) *Router {
	return &Router{
		matrix:      m,
		llm:         l,
		memory:      mem,
		billing:     b,
		cfg:         cfg,
		logger:      logger,
		quota:       quota,
		rateManager: rateManager,
		bootTime:    time.Now(),
	}
}
