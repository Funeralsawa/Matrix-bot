package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"nozomi/internal/billing"
	"nozomi/internal/config"
	"nozomi/internal/llm"
	"nozomi/internal/logger"
	"nozomi/internal/matrix"
	"nozomi/internal/memory"
	"nozomi/internal/quota"

	"google.golang.org/genai"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type Router struct {
	matrix   *matrix.Client
	llm      *llm.Client
	memory   *memory.Manager
	billing  *billing.System
	cfg      *config.BotConfig
	logger   *logger.Logger
	quota    *quota.Manager
	bootTime time.Time // 用于过滤启动前的历史陈旧消息
}

func NewRouter(m *matrix.Client, l *llm.Client, mem *memory.Manager, b *billing.System, cfg *config.BotConfig, logger *logger.Logger, quota *quota.Manager) *Router {
	return &Router{
		matrix:   m,
		llm:      l,
		memory:   mem,
		billing:  b,
		cfg:      cfg,
		logger:   logger,
		quota:    quota,
		bootTime: time.Now(),
	}
}

// HandleMessage 专门处理 m.room.message 事件
func (r *Router) HandleMessage(ctx context.Context, evt *event.Event) {
	// 1. 无视启动前的消息和自己发的消息
	if time.UnixMilli(evt.Timestamp).Before(r.bootTime) || evt.Sender == r.cfg.Client.UserID {
		return
	}

	// 2. 委托 Matrix 领域解析消息
	msgCtx, err := r.matrix.ParseMessage(ctx, evt)
	if err != nil {
		return
	}

	roomID := evt.RoomID.String()
	sender := evt.Sender.String()

	// 4. 调用 Matrix 领域获取房间人数，判定聊天类型
	memberCount, err := r.matrix.GetRoomMemberCount(ctx, roomID)
	if err != nil {
		return
	}
	isGroup := memberCount > 2
	if isGroup {
		// 群聊逻辑
		msgCtx.Text = fmt.Sprintf("%s 发言：%s\n", sender, msgCtx.Text)
	} else {
		// 私聊逻辑
		msgCtx.IsMentioned = true

		// 私聊特殊的连续发图合并逻辑 (通过识别特定的无配文占位符)
		if msgCtx.ImagePart != nil && msgCtx.Text == "(发送了一张图片)" {
			// 委托 Memory 领域暂存图片，并拿回当前图片总数
			imgCount := r.memory.AddPrivateImageCache(roomID, msgCtx.ImagePart)
			// 发送阻断提示
			str := fmt.Sprintf("收到 %d 张图。请在 5 分钟内补充文字说明。", imgCount)
			_ = r.matrix.SendText(ctx, id.RoomID(roomID), str)
			return
		}
	}

	// 5. 委托 Memory 领域：记录群友说的话，并取出极其安全的上下文深拷贝
	history := r.memory.AddUserMsgAndLoad(roomID, msgCtx.Text, msgCtx.ImagePart)

	// 6. 如果没有关键字，只记入记忆
	if !msgCtx.IsMentioned {
		return
	}

	// 7. 开启独立工作协程，不阻塞 Matrix 的主接收线程
	go func(safeHistory []*genai.Content, text string, sender id.UserID, rID id.RoomID) {
		bgCtx := context.Background()

		// 判断联网次数是否耗光
		var dynamicConfig *genai.GenerateContentConfig
		if r.quota.CheckAndGetRemaining() <= 0 {
			dynamicConfig = r.llm.GetConfigWithoutSearch()
		}

		// 委托 LLM 领域：发起思考与生成
		res, usage, err := r.llm.Generate(bgCtx, safeHistory, dynamicConfig)
		if err != nil {
			str := "用户：" + evt.Sender.String() + "\n"
			str += "房间：" + evt.RoomID.String() + "\n"
			str += "请求：" + msgCtx.Text + "\n"
			str += "时间：" + time.UnixMilli(evt.Timestamp).Format("2006-01-02 15:04:05") + "\n"

			errMsg := err.Error()
			isLocalTimeout := errors.Is(err, context.DeadlineExceeded)
			isRemoteTimeout := strings.Contains(errMsg, "DEADLINE_EXCEEDED") || strings.Contains(errMsg, "504")
			if isLocalTimeout || isRemoteTimeout {
				str += "大模型调用超时！"
				_ = r.matrix.SendText(ctx, evt.RoomID, "Network congestion.Please try again later.")
				_ = r.logger.Log("error", "Call LLM time out.", logger.Options{})
			} else {
				_ = r.matrix.SendText(ctx, evt.RoomID, "Sorry, I need rest.")
				_ = r.logger.Log("error", fmt.Sprintf("Gemini meet an error: %s", err.Error()), logger.Options{})

				str += "错误：" + err.Error()
			}

			r.matrix.SendToLogRoom(ctx, str)
			return
		}

		if res.UsedSearch {
			// 委托 Quota 领域：扣减一次额度
			r.quota.Consume()
		}

		// 记录日志
		tokenConsume := fmt.Sprintf(
			" | 输入%d 输出%d 总计消耗%d | %v",
			usage.Input,
			usage.Output,
			usage.Think+usage.Input+usage.Output,
			res.CostTime,
		)
		_ = r.logger.Log("bot", msgCtx.Text+tokenConsume, logger.Options{UserID: evt.Sender.String(), RoomID: evt.RoomID.String()})

		// 委托 Billing 领域：安全地记账
		r.billing.Record(usage.Input, usage.Output, usage.Think)
		if r.billing.CheckAlarm(usage.Input + usage.Output + usage.Think) {
			str := "用量警报！\n"
			str += "用户：" + evt.Sender.String() + "\n"
			str += "房间：" + evt.RoomID.String() + "\n"
			str += "请求：" + msgCtx.Text + "\n"
			str += "时间: " + time.UnixMilli(evt.Timestamp).Format("2006-01-02 15:04:05") + "\n"
			str += "Token 账单单次达到警报值！\n"
			str += tokenConsume
			errs := r.matrix.SendToLogRoom(bgCtx, str)
			for _, err := range errs {
				r.logger.Log("error", "Sending log to log-room error: "+err.Error(), logger.Options{})
			}
		}

		// 9. 委托 Memory 领域：将大模型的纯净回复写入记忆
		r.memory.AddModelMsg(roomID, res.CleanParts)

		// 10. 委托 Matrix 领域：将富文本渲染并发送到房间
		err = r.matrix.SendMarkdown(bgCtx, rID, res.RawText)
		if err != nil {
			_ = r.logger.Log("error", "Failed to send message to room: "+err.Error(), logger.Options{})
		}

	}(history, msgCtx.Text, evt.Sender, evt.RoomID)
}

// HandleMember 专门处理 m.room.member 事件（原 evtMember.go 的终极进化版）
func (r *Router) HandleMember(ctx context.Context, evt *event.Event) {
	memberEvent := evt.Content.AsMember()
	if memberEvent == nil || evt.StateKey == nil || *evt.StateKey != string(r.cfg.Client.UserID) {
		return
	}

	switch memberEvent.Membership {
	case event.MembershipInvite:
		// 委托 Matrix 领域：自动同意加入房间
		err := r.matrix.JoinRoom(ctx, evt.RoomID)
		if err == nil {
			r.matrix.SendText(ctx, evt.RoomID, "你好，我是希。")
			_ = r.logger.Log("info", "Auto accept room invite: "+evt.RoomID.String(), logger.Options{})
		}
	case event.MembershipLeave, event.MembershipBan:
		// 委托 Memory 领域：被踢出或退出时，物理清除该房间的所有记忆
		r.memory.Delete(evt.RoomID.String())
		_ = r.logger.Log("info", "Auto clear memory of didn't join room: "+evt.RoomID.String(), logger.Options{})
	}
}
