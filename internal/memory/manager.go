package memory

import (
	"fmt"
	"sync"
	"time"

	"google.golang.org/genai"
)

type ImageCacheItem struct {
	sync.Mutex
	Parts      []*genai.Part
	ExpireTime time.Time
}

type Manager struct {
	chatMemory        sync.Map // 格式: map[string][]*genai.Content
	privateImageCache sync.Map // 格式: map[string]*ImageCacheItem
	roomLocks         sync.Map // 格式: map[string]*sync.Mutex
	maxMemoryLength   int      // 记忆最大长度（图纸配置）
}

// 构造函数
func NewManager(maxLength int) *Manager {
	return &Manager{
		maxMemoryLength: maxLength,
	}
}

// 获取房间锁，安全控制并发
func (m *Manager) getRoomLock(roomID string) *sync.Mutex {
	lockObj, _ := m.roomLocks.LoadOrStore(roomID, &sync.Mutex{})
	return lockObj.(*sync.Mutex)
}

// Load 取出当前房间的对话记忆（深拷贝，防止指针踩踏）
func (m *Manager) Load(roomID string) []*genai.Content {
	m.getRoomLock(roomID).Lock()
	defer m.getRoomLock(roomID).Unlock()

	var history []*genai.Content
	if val, ok := m.chatMemory.Load(roomID); ok {
		rawHistory := val.([]*genai.Content)
		// 深拷贝逻辑
		history = make([]*genai.Content, len(rawHistory))
		for i, h := range rawHistory {
			partsCopy := make([]*genai.Part, len(h.Parts))
			copy(partsCopy, h.Parts)
			history[i] = &genai.Content{
				Role:  h.Role,
				Parts: partsCopy,
			}
		}
	}
	return history
}

// AddUserMsgAndLoad 记录群友的新发言，并返回用于大模型调用的深拷贝记忆
func (m *Manager) AddUserMsgAndLoad(roomID string, text string, imgPart *genai.Part) []*genai.Content {
	m.getRoomLock(roomID).Lock()
	defer m.getRoomLock(roomID).Unlock()

	var history []*genai.Content
	if val, ok := m.chatMemory.Load(roomID); ok {
		history = val.([]*genai.Content)
	}

	// 组装当前这句发言的 Parts
	var currentParts []*genai.Part
	cachedImgs := m.pullPrivateImageCache(roomID)
	now := time.Now().Format("2006-01-02 15:04:05")
	if len(cachedImgs) > 0 {
		label := fmt.Sprintf("(%s)发送了一组图片：", now)
		currentParts = append(currentParts, genai.Text(label)[0].Parts[0])
		currentParts = append(currentParts, cachedImgs...)
	}
	if text != "" {
		text = fmt.Sprintf("(%s) %s", now, text)
		currentParts = append(currentParts, genai.Text(text)[0].Parts[0])
	}
	if imgPart != nil {
		currentParts = append(currentParts, imgPart)
	}

	if len(currentParts) == 0 {
		return m.deepCopy(history)
	}

	// 防止 Gemini 连续 user 报错：合并同类项
	historyLen := len(history)
	if historyLen > 0 && history[historyLen-1].Role == "user" {
		// 上一句话是人类说的，直接把新的文本塞进上一个 user 的包裹里
		history[historyLen-1].Parts = append(history[historyLen-1].Parts, currentParts...)
	} else {
		// 上一句话是大模型的，这是一个全新的对话，创建一个新的 user 节点
		userMsg := &genai.Content{
			Role:  "user",
			Parts: currentParts,
		}
		history = append(history, userMsg)
	}

	// 裁切并保存到原始内存
	history = m.cleanup(history)
	m.chatMemory.Store(roomID, history)

	// 返回一份深拷贝给独立协程，防止指针被并发踩踏
	return m.deepCopy(history)
}

// AddModelMsg 将大模型的纯净回复写入记忆
func (m *Manager) AddModelMsg(roomID string, cleanParts []*genai.Part) {
	if len(cleanParts) == 0 {
		return
	}

	m.getRoomLock(roomID).Lock()
	defer m.getRoomLock(roomID).Unlock()

	var history []*genai.Content
	if val, ok := m.chatMemory.Load(roomID); ok {
		history = val.([]*genai.Content)
	}

	safeModelMsg := &genai.Content{
		Role:  "model",
		Parts: cleanParts,
	}

	history = append(history, safeModelMsg)
	history = m.cleanup(history)
	m.chatMemory.Store(roomID, history)
}

// CleanExpiredImages 扫描并清理过期的私聊图片缓存，返回已清理的 roomID 列表供外层通知
func (m *Manager) CleanExpiredImages() []string {
	var expiredRooms []string
	now := time.Now()

	m.privateImageCache.Range(func(key, val any) bool {
		roomID := key.(string)
		cache := val.(*ImageCacheItem)

		cache.Lock()
		isExpired := now.After(cache.ExpireTime)
		cache.Unlock()

		if isExpired {
			m.privateImageCache.Delete(roomID)
			expiredRooms = append(expiredRooms, roomID)
		}
		return true
	})
	return expiredRooms
}

// RetainOnly 对比活跃房间列表，把不在列表里的幽灵房间记忆抹除
func (m *Manager) RetainOnly(activeRooms []string) {
	validRooms := make(map[string]bool)
	for _, r := range activeRooms {
		validRooms[r] = true
	}

	m.chatMemory.Range(func(key, val any) bool {
		roomID := key.(string)
		if !validRooms[roomID] {
			m.chatMemory.Delete(key)
			m.privateImageCache.Delete(key)
		}
		return true
	})
}

// AddPrivateImageCache 专门用于私聊：暂存没有配文的图片，并返回当前已暂存的数量
func (m *Manager) AddPrivateImageCache(roomID string, imgPart *genai.Part) int {
	val, _ := m.privateImageCache.LoadOrStore(roomID, &ImageCacheItem{})
	cache := val.(*ImageCacheItem)

	cache.Lock()
	defer cache.Unlock()

	// 追加新图之前先判断是否有过期旧图缓存
	if time.Now().After(cache.ExpireTime) && len(cache.Parts) > 0 {
		cache.Parts = nil
	}
	cache.Parts = append(cache.Parts, imgPart)
	cache.ExpireTime = time.Now().Add(5 * time.Minute)

	return len(cache.Parts)
}

// pullPrivateImageCache 取出并清空暂存的私聊图片，准备与文字合并
func (m *Manager) pullPrivateImageCache(roomID string) []*genai.Part {
	val, ok := m.privateImageCache.Load(roomID)
	if !ok {
		return nil
	}

	cache := val.(*ImageCacheItem)
	cache.Lock()
	defer cache.Unlock()

	var parts []*genai.Part
	if time.Now().Before(cache.ExpireTime) && len(cache.Parts) > 0 {
		parts = append(parts, cache.Parts...)
	}
	cache.Parts = nil // 取出后立刻清空
	m.privateImageCache.Delete(roomID)

	return parts
}

// Store 将清理过的最新记忆安全地写回内存
func (m *Manager) Store(roomID string, history []*genai.Content) {
	m.getRoomLock(roomID).Lock()
	defer m.getRoomLock(roomID).Unlock()

	// 调用内部的私有滑动窗口清理函数
	cleanHistory := m.cleanup(history)
	m.chatMemory.Store(roomID, cleanHistory)
}

// 清除指定房间的记忆 (比如退群或空房间时调用)
func (m *Manager) Delete(roomID string) {
	m.chatMemory.Delete(roomID)
	m.privateImageCache.Delete(roomID)
}

// deepCopy 深拷贝工具
func (m *Manager) deepCopy(rawHistory []*genai.Content) []*genai.Content {
	history := make([]*genai.Content, len(rawHistory))
	for i, h := range rawHistory {
		partsCopy := make([]*genai.Part, len(h.Parts))
		copy(partsCopy, h.Parts)
		history[i] = &genai.Content{
			Role:  h.Role,
			Parts: partsCopy,
		}
	}
	return history
}

// 获取记忆长度
func getHistoryLen(history []*genai.Content) int {
	totalLength := 0
	for _, content := range history {
		if content.Role == "user" {
			totalLength += len(content.Parts)
		} else {
			totalLength += 1
		}
	}
	return totalLength
}

// 滑动窗口裁剪逻辑
func (m *Manager) cleanup(history []*genai.Content) []*genai.Content {
	totalLength := getHistoryLen(history)
	for totalLength > m.maxMemoryLength {
		if history[0].Role == "user" && len(history[0].Parts) > 1 {
			history[0].Parts = history[0].Parts[1:]
		} else {
			history = history[1:]
		}
		totalLength--
	}
	for len(history) > 0 && history[0].Role == "model" {
		if len(history) == 1 {
			history = nil
		} else {
			history = history[1:]
		}
	}
	return history
}
