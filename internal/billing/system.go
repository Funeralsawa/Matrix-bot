package billing

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type System struct {
	mu             sync.Mutex   // 保护账单的专属互斥锁
	usage          ConsumeToken // 真正的账单数据
	timeLog        TimeLog      // 游标时间
	alarmThreshold int32        // 报警线
	workdir        string       // 工作目录，用于落盘
}

// 构造函数
func NewSystem(alarmLine int32, workdir string) *System {
	s := &System{
		alarmThreshold: alarmLine,
		workdir:        workdir,
	}
	s.load()
	return s
}

// Record 安全地记录一笔新的消耗
func (s *System) Record(input, output, think int32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.usage.Day.Add(input, output, think)
	s.usage.Month.Add(input, output, think)
	s.usage.Year.Add(input, output, think)

	s.saveTokenUsage()
}

// GetUsage 返回当前的完整账单拷贝
func (s *System) GetUsage() ConsumeToken {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.usage
}

// CheckAlarm 判断单次消耗是否超过了报警线
func (s *System) CheckAlarm(total int32) bool {
	return total >= s.alarmThreshold
}

// CheckAndReset 核对跨日/月/年时间，执行结算
// 只把结算出的报表文本组装成切片返回
func (s *System) CheckAndReset() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	// 如果时间是空的，初始化游标并退出
	if s.timeLog.Time.IsZero() {
		s.timeLog.Time = now
		s.saveTimeLog()
		return nil
	}

	isNewDay := now.Format("2006-01-02") != s.timeLog.Time.Format("2006-01-02")
	isNewMonth := now.Format("2006-01") != s.timeLog.Time.Format("2006-01")
	isNewYear := now.Format("2006") != s.timeLog.Time.Format("2006")

	if !isNewDay {
		return nil
	}

	var reports []string

	if isNewDay {
		reports = append(reports, fmt.Sprintf("Token 日账单：\n\tInput: %d\n\tOutput: %d\n\tThink: %d\n\tTotal: %d",
			s.usage.Day.Input, s.usage.Day.Output, s.usage.Day.Think, s.usage.Day.CountTotal()))
		s.usage.Day.ResetTokenUsage()
	}

	if isNewMonth {
		reports = append(reports, fmt.Sprintf("Token 月账单：\n\tInput: %d\n\tOutput: %d\n\tThink: %d\n\tTotal: %d",
			s.usage.Month.Input, s.usage.Month.Output, s.usage.Month.Think, s.usage.Month.CountTotal()))
		s.usage.Month.ResetTokenUsage()
	}

	if isNewYear {
		reports = append(reports, fmt.Sprintf("Token 年账单：\n\tInput: %d\n\tOutput: %d\n\tThink: %d\n\tTotal: %d",
			s.usage.Year.Input, s.usage.Year.Output, s.usage.Year.Think, s.usage.Year.CountTotal()))
		s.usage.Year.ResetTokenUsage()
	}

	s.timeLog.Time = now
	s.saveTimeLog()
	s.saveTokenUsage()

	return reports
}

func (s *System) load() {
	timePath := filepath.Join(s.workdir, "data", "time.json")
	if data, err := os.ReadFile(timePath); err == nil {
		_ = json.Unmarshal(data, &s.timeLog)
	}

	tokenPath := filepath.Join(s.workdir, "data", "token_usage.json")
	if data, err := os.ReadFile(tokenPath); err == nil {
		_ = json.Unmarshal(data, &s.usage)
	}
}

func (s *System) saveTimeLog() {
	path := filepath.Join(s.workdir, "data", "time.json")
	if data, err := json.MarshalIndent(s.timeLog, "", "\t"); err == nil {
		_ = os.WriteFile(path, data, 0644)
	}
}

func (s *System) saveTokenUsage() {
	path := filepath.Join(s.workdir, "data", "token_usage.json")
	if data, err := json.MarshalIndent(s.usage, "", "\t"); err == nil {
		_ = os.WriteFile(path, data, 0644)
	}
}
