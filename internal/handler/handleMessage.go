package handler

import (
	"context"
	"errors"
	"fmt"
	"nozomi/internal/logger"
	"regexp"
	"strings"
	"time"

	"google.golang.org/genai"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// HandleMessage 专门处理群聊信息事件，包括 m.room.message 和 event.EventSticker
func (r *Router) HandleMessage(ctx context.Context, evt *event.Event) {
	// 无视启动前的消息和自己发的消息
	if time.UnixMilli(evt.Timestamp).Before(r.bootTime) || evt.Sender == r.cfg.Client.UserID {
		return
	}

	// 房间类型判断
	memberCount, err := r.matrix.GetRoomMemberCount(ctx, evt.RoomID.String())
	if err != nil {
		r.logger.Log("error", "Failed to get room member count: "+err.Error(), logger.Options{})
		return
	}
	isGroup := memberCount > 2

	// 解析消息
	msgCtxs, err := r.matrix.ParseMessage(ctx, evt, 1)
	if err != nil {
		if err.Error() != "not a message event" && err.Error() != "Not support of gif image." {
			str := "user: " + evt.Sender.String() + "\n"
			str += "room: " + evt.RoomID.String() + "\n"
			str += "time: " + time.UnixMilli(evt.Timestamp).Format("2006-01-02 15:04:05") + "\n"
			str += "error: " + "Failed to parse message: " + err.Error()

			_ = r.matrix.SendText(ctx, evt.RoomID, "Sorry, I need rest.Pls try again later.")
			_ = r.logger.Log("error", "Failed to parse message: "+err.Error(), logger.Options{})
			errs := r.matrix.SendToLogRoom(ctx, str)
			for _, err := range errs {
				str := "Sending log to log-room error: " + err.Error()
				_ = r.logger.Log("error", str, logger.Options{})
			}
		}
		return
	}
	msgCtxsLen := len(msgCtxs)
	if msgCtxsLen == 0 {
		return
	}

	currentCtx := msgCtxs[len(msgCtxs)-1]

	// 指令检测
	if r.CheckCommand(ctx, currentCtx.Text, evt.RoomID, evt.Sender) {
		return
	}

	finalText, finalImages := r.FormatMessageContexts(msgCtxs, isGroup)

	roomID := evt.RoomID.String()
	sender := evt.Sender.String()

	// 私聊逻辑
	if !isGroup {
		currentCtx.IsMentioned = true

		// 私聊特殊的连续发图合并逻辑
		isPureImageOrSticker := false
		if currentCtx.ImagePart != nil {
			matched, _ := regexp.MatchString(`^\(Send a (picture|sticker)[^)]*\)\s*$`, currentCtx.Text)
			isPureImageOrSticker = matched
		}

		if isPureImageOrSticker {
			// 暂存当前这张图
			imgCount := r.memory.AddPrivateImageCache(roomID, currentCtx.ImagePart)
			str := fmt.Sprintf("Receive picture %d.Please provide a written description within 5 minutes.", imgCount)
			_ = r.matrix.SendText(ctx, id.RoomID(roomID), str)
			return
		}
	}

	// 记录群友说的话，并取出安全的上下文深拷贝
	history := r.memory.AddUserMsgAndLoad(roomID, isGroup, finalText, finalImages...)

	// 如果没有关键字，只记入记忆
	if !currentCtx.IsMentioned {
		return
	}

	if currentCtx.IsUnsupportedImageType {
		_ = r.matrix.SendText(ctx, evt.RoomID, "Not support this type of image.")
		return
	}

	// 高频拦截
	if !r.rateManager.AllowRequest(sender) {
		str := "Intercepting high-frequency requests：\n"
		str += "room: " + roomID + "\n"
		str += "user: " + sender
		r.matrix.SendText(ctx, evt.RoomID, "Sorry, I need rest.Please try again later.")
		errs := r.matrix.SendToLogRoom(ctx, str)
		for _, err := range errs {
			r.logger.Log("error", "Failed to send log to log-room: "+err.Error(), logger.Options{})
		}
		r.logger.Log("info", "Intercepted abnormally high frequency requests.", logger.Options{})
		return
	}

	// 开启独立工作协程，不阻塞 Matrix 的主接收线程
	go func(safeHistory []*genai.Content, text string, sender id.UserID, rID id.RoomID, isGroup bool) {
		bgCtx := context.Background()

		// 发送已读回执
		err := r.matrix.MarkRead(bgCtx, rID, evt.ID)
		if err != nil {
			str := fmt.Sprintf("Failed to send read receipt to room %v: %v", rID, err)
			r.logger.Log("error", str, logger.Options{})
			r.matrix.SendToLogRoom(bgCtx, str)
		}

		// 模拟人类输入
		done := make(chan struct{})
		go func() {
			timer := time.NewTimer(2 * time.Second)
			defer timer.Stop()

			select {
			case <-done:
				return
			case <-timer.C:
				_ = r.matrix.UserTyping(bgCtx, rID, true, r.cfg.Model.TimeOutWhen)
				<-done
				_ = r.matrix.UserTyping(bgCtx, rID, false, 0)
			}
		}()
		defer close(done)

		// Call Agent Loop
		agentResponse := r.RunAgentLoop(bgCtx, safeHistory, sender, rID)
		if agentResponse.Err != nil {
			str := "user: " + sender.String() + "\n"
			str += "room: " + rID.String() + "\n"
			str += "request: " + text + "\n"
			str += "time: " + time.UnixMilli(evt.Timestamp).Format("2006-01-02 15:04:05") + "\n"

			errMsg := agentResponse.Err.Error()

			isLocalTimeout := errors.Is(err, context.DeadlineExceeded)
			isRemoteTimeout := strings.Contains(errMsg, "DEADLINE_EXCEEDED") || strings.Contains(errMsg, "504")
			if isLocalTimeout || isRemoteTimeout {
				str += "LLM call timed out!"
				_ = r.matrix.SendText(bgCtx, rID, "Network congestion.Please try again later.")
				_ = r.logger.Log("error", "Call LLM time out.", logger.Options{})
			} else {
				_ = r.matrix.SendText(bgCtx, rID, "Sorry, I need rest.Please try again later")
				_ = r.logger.Log("error", fmt.Sprintf("Gemini meet an error: %s", errMsg), logger.Options{})

				str += "error: " + errMsg
			}

			errs := r.matrix.SendToLogRoom(bgCtx, str)
			for _, err := range errs {
				str := "Sending log to log-room error: " + err.Error()
				_ = r.logger.Log("error", str, logger.Options{})
				r.matrix.SendToLogRoom(ctx, str)
			}
			return
		}

		// 记录消耗日志
		r.agentLog(agentResponse, text, sender, rID)

		// 安全地记账
		r.billingRecord(bgCtx, text, agentResponse, sender, rID, time.UnixMilli(evt.Timestamp))

		// 确认是否需要执行记忆回传算法
		nowHistoryLen := r.memory.GetHistoryLen(safeHistory)
		needMemoryRetrospection := nowHistoryLen >= r.cfg.Client.MaxMemoryLength && !isGroup
		if needMemoryRetrospection && r.memory.TryLockRoomSummarization(rID) {
			oldH, summarizedPartCount := r.memory.GetOldHistory(safeHistory)
			go r.ExecuteMemoryRetrospection(oldH, summarizedPartCount, rID)
		}

		// 将大模型的纯净回复写入记忆
		if agentResponse.LastResult.CleanParts != nil {
			r.memory.AddModelMsg(rID.String(), isGroup, agentResponse.LastResult.CleanParts)
		}

		// 将富文本渲染并发送到房间
		err = r.matrix.SendMarkdownWithMath(bgCtx, rID, agentResponse.LastResult.RawText)
		if err != nil {
			str := "user: " + sender.String() + "\n"
			str += "room: " + rID.String() + "\n"
			str += "request: " + text + "\n"
			str += "time: " + time.UnixMilli(evt.Timestamp).Format("2006-01-02 15:04:05") + "\n"
			str += "error: " + "Failed to send rich message to room: " + err.Error()
			_ = r.matrix.SendText(bgCtx, rID, "sorry, I need rest, please try again later.")
			_ = r.logger.Log("error", "Failed to send rich message to room: "+err.Error(), logger.Options{})
			errs := r.matrix.SendToLogRoom(bgCtx, str)
			for _, err := range errs {
				str := "Sending log to log-room error: " + err.Error()
				_ = r.logger.Log("error", str, logger.Options{})
			}
			return
		}
	}(history, currentCtx.Text, evt.Sender, evt.RoomID, isGroup)
}
