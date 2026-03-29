package bot

import (
	"nozomi/internal/logger"
	"time"

	"maunium.net/go/mautrix/id"
)

// 定期清理过期的私聊图片缓存
func startImageCacheCleanupTask() {
	// 每 1 分钟检查一次就足够了
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		<-ticker.C
		now := time.Now()

		privateImageCache.Range(func(key, val any) bool {
			roomID := key.(string)
			cache := val.(*ImageCacheItem)

			cache.Lock()
			// 判断现在的时间是否已经超过了它设定的过期时间
			isExpired := now.After(cache.ExpireTime)
			cache.Unlock()

			if isExpired {
				privateImageCache.Delete(roomID)
				_ = logger.Log("info", "Auto cleared expired image cache for room: "+roomID, logger.Options{})
				_, _ = client.SendText(ctx, id.RoomID(roomID), "你发送的图片已超时，我先忘记了哦。")
			}
			return true
		})
	}
}
