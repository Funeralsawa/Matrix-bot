package quota

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type SearchQuota struct {
	Month string `json:"month"`
	Count int    `json:"count"`
}

type Manager struct {
	mu          sync.Mutex
	data        SearchQuota
	maxPerMonth int
	workdir     string
}

func NewManager(maxPerMonth int, workdir string) *Manager {
	m := &Manager{
		maxPerMonth: maxPerMonth,
		workdir:     workdir,
	}
	m.load()
	return m
}

// CheckAndGetRemaining 检查是否跨月，并返回剩余额度
func (m *Manager) CheckAndGetRemaining() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	nowMonth := time.Now().Format("2006-01")
	if m.data.Month != nowMonth {
		m.data.Month = nowMonth
		m.data.Count = m.maxPerMonth
		m.save()
	}
	return m.data.Count
}

// Consume 安全地扣除一次额度并持久化
func (m *Manager) Consume() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.data.Count > 0 {
		m.data.Count--
		m.save()
	}
}

func (m *Manager) load() {
	path := filepath.Join(m.workdir, "data", "search_quota.json")
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &m.data)
	}
}

func (m *Manager) save() {
	path := filepath.Join(m.workdir, "data", "search_quota.json")
	if b, err := json.MarshalIndent(m.data, "", "\t"); err == nil {
		_ = os.WriteFile(path, b, 0644)
	}
}
