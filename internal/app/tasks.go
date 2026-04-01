package app

import (
	"context"
	"fmt"
	"time"

	"nozomi/internal/logger"

	"maunium.net/go/mautrix/id"
)

// StartBackgroundTasks 启动所有的GC和账单检查线程
func (a *App) StartBackgroundTasks(ctx context.Context) {
	go a.imageCacheCleanupLoop(ctx)
	go a.roomCleanupLoop(ctx)
	go a.nonExistRoomCleanupLoop(ctx)
	go a.billingCheckLoop(ctx)
	go a.profileKeeperLoop(ctx)
}

// 清理私信房间的图片记忆缓存
func (a *App) imageCacheCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			expiredRooms := a.Memory.CleanExpiredImages()

			for _, roomID := range expiredRooms {
				_ = a.Matrix.SendText(ctx, id.RoomID(roomID), "你发送的图片已超时，我先忘记了哦。")
				_ = a.Logger.Log("info", "Auto cleared expired image cache for room: "+roomID, logger.Options{})
			}
		}
	}
}

// 清理无人空房间
func (a *App) roomCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rooms, err := a.Matrix.GetJoinedRooms(ctx)
			if err != nil {
				_ = a.Logger.Log("error", "Failed to get room list when begin to cleanup rooms."+err.Error(), logger.Options{})
				continue
			}

			for _, roomID := range rooms {
				count, err := a.Matrix.GetRoomMemberCount(ctx, roomID)
				if err != nil {
					_ = a.Logger.Log("error", fmt.Sprintf("Failed to get member of the room %s with error %s", roomID, err.Error()), logger.Options{})
					continue
				}

				if count <= 1 {
					if err := a.Matrix.LeaveRoom(ctx, roomID); err == nil {
						a.Memory.Delete(roomID)
						_ = a.Logger.Log("info", "Exit the empty room sucessfully and cleared memory: "+roomID, logger.Options{})
					} else {
						a.Matrix.SendToLogRoom(ctx, "Failed to exit the empty room: "+roomID)
					}
				}
			}
			_ = a.Logger.Log("info", "The empty room cleanup task has done.", logger.Options{})
		}
	}
}

// 清理不存在房间的记忆
func (a *App) nonExistRoomCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(12 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rooms, err := a.Matrix.GetJoinedRooms(ctx)
			if err != nil {
				_ = a.Logger.Log("error", "Failed to retrieve the list of joined rooms when clear non-exist room Memories", logger.Options{})
				continue
			}
			// 将存活名单甩给内存领域，让它把不在名单里的幽灵全杀掉
			a.Memory.RetainOnly(rooms)
			_ = a.Logger.Log("info", "Auto cleared memory for non-exist rooms.", logger.Options{})
		}
	}
}

// 账单巡检逻辑
func (a *App) billingCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 1. 让账单领域去算账，如果有跨天/月，它会交回 []string 格式的报表
			reports := a.Billing.CheckAndReset()

			// 2. 调度 Matrix 领域，把报表投递到所有的日志管理群
			for _, report := range reports {
				a.Matrix.SendToLogRoom(ctx, report)
			}
		}
	}
}

// 对应原 botInfo.go 的守护进程
func (a *App) profileKeeperLoop(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	// 极其严谨：程序刚启动时，强制执行一次查岗
	a.checkAndRestoreProfile()

	for {
		select {
		case <-ctx.Done():
			return // 收到系统退出信号，保安下班
		case <-ticker.C:
			a.checkAndRestoreProfile()
		}
	}
}

// 核心自愈动作：核对并修复
func (a *App) checkAndRestoreProfile() {
	cfgName := a.Config.Client.DisplayName
	cfgAvatar := a.Config.Client.AvatarURL

	// 如果配置文件里压根没填名字和头像，就不用巡检了
	if cfgName == "" && cfgAvatar == "" {
		return
	}

	// 1. 调用 Matrix 领域：获取当前服务器上的真实情况
	currentName, currentAvatar, err := a.Matrix.GetProfile()
	if err != nil {
		_ = a.Logger.Log("error", "ProfileKeeper 获取资料失败: "+err.Error(), logger.Options{})
		return
	}

	needsUpdate := false
	updateName := ""
	updateAvatar := ""

	// 2. 比对昵称：发现被篡改或丢失，准备恢复
	if cfgName != "" && currentName != cfgName {
		needsUpdate = true
		updateName = cfgName
	}

	// 3. 比对头像：发现头像被清空或不一致，准备恢复
	if cfgAvatar != "" && currentAvatar != cfgAvatar {
		needsUpdate = true
		updateAvatar = cfgAvatar
	}

	// 4. 执行修复：委托 Matrix 领域重新把脸和名字挂上去
	if needsUpdate {
		err := a.Matrix.SetProfile(updateName, updateAvatar)
		if err != nil {
			_ = a.Logger.Log("error", "ProfileKeeper 恢复资料失败: "+err.Error(), logger.Options{})
		} else {
			_ = a.Logger.Log("info", "ProfileKeeper 成功触发资料自愈！", logger.Options{})
		}
	}
}
