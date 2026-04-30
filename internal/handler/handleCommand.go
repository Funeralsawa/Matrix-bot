package handler

import (
	"context"
	"nozomi/tools"
	"strings"

	"maunium.net/go/mautrix/id"
)

func (r *Router) CheckCommand(ctx context.Context, text string, roomID id.RoomID, sender id.UserID) bool {
	if r.checkIsPendingTask(ctx, text, roomID, sender) {
		return true
	}
	return false
}

func (r *Router) checkIsPendingTask(ctx context.Context, text string, roomID id.RoomID, sender id.UserID) bool {
	if strings.HasPrefix(text, "/YES ") {
		taskID := strings.TrimSpace(strings.TrimPrefix(text, "/YES "))

		// 查找是否有这个 task 在等待
		if ch, ok := r.pendingApprovals.Load(tools.CommandTask{RoomID: roomID, SenderID: sender, TaskID: taskID}); ok {
			waitChan := ch.(chan bool)
			r.matrix.SendText(ctx, roomID, "Authorized task: "+taskID)
			waitChan <- true // 发送放行信号，唤醒等待的协程
		} else {
			r.matrix.SendText(ctx, roomID, "Invalid or expired task ID")
		}
		return true
	}

	if strings.HasPrefix(text, "/NO ") {
		taskID := strings.TrimSpace(strings.TrimPrefix(text, "/NO "))
		if ch, ok := r.pendingApprovals.Load(tools.CommandTask{RoomID: roomID, SenderID: sender, TaskID: taskID}); ok {
			waitChan := ch.(chan bool)
			r.matrix.SendText(ctx, roomID, "Task rejected: "+taskID)
			waitChan <- false // 发送拒绝信号
		}
		return true
	}

	return false
}
