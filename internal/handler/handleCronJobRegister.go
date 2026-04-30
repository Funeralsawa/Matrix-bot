package handler

import (
	"context"
	"fmt"
	"nozomi/internal/logger"
	"nozomi/tools"
	"time"

	"github.com/robfig/cron/v3"
	"google.golang.org/genai"
)

func (r *Router) HandleCronJobRegister(task tools.CronTask) (cron.EntryID, error) {
	cronID, err := tools.CronEngine.AddFunc(task.CronExpr, func() {
		bgCtx := context.Background()

		// 构造系统唤醒指令
		triggerMsg := fmt.Sprintf("[System Cron Trigger]: Scheduled task has been triggered:\n%s", task.TaskPrompt)

		// 记录当前房间是否为群聊
		memberCount, _ := r.matrix.GetRoomMemberCount(bgCtx, task.RoomID.String())
		isGroup := memberCount > 2

		// 构建 history []*genai.Content
		history := genai.Text(triggerMsg)

		// 唤醒 Agent
		agentResponseChan := make(chan *AgentResponse)
		go func() {
			agentResponse := r.RunAgentLoop(bgCtx, history, r.cfg.Client.UserID, task.RoomID)
			agentResponseChan <- agentResponse
		}()
		agentResponse, _ := <-agentResponseChan
		close(agentResponseChan)
		if agentResponse.Err != nil {
			errStr := fmt.Sprintf("Error occurred when execute cron job(%s): %v", task.UUID, agentResponse.Err)
			r.logger.Log("error", errStr, logger.Options{})
			r.matrix.SendText(bgCtx, task.RoomID, errStr)
			r.matrix.SendToLogRoom(bgCtx, errStr)
			return
		}

		// 记录 token 消耗
		execTime := time.Now()
		r.agentLog(agentResponse, triggerMsg, task.Sender, task.RoomID)
		r.billingRecord(bgCtx, triggerMsg, agentResponse, task.Sender, task.RoomID, execTime)

		// 将纯净回复写入记忆
		if agentResponse.LastResult.CleanParts != nil {
			r.memory.AddModelMsg(task.RoomID.String(), isGroup, agentResponse.LastResult.CleanParts)
		}

		// 结果发送回房间
		err := r.matrix.SendMarkdownWithMath(bgCtx, task.RoomID, agentResponse.LastResult.RawText)
		if err != nil {
			str := fmt.Sprintf("Failed to send cron job(%s) result.", task.UUID)
			logStr := fmt.Sprintf("Failed to send cron job(%s) result to room %v: %v", task.UUID, task.RoomID, err)
			_ = r.matrix.SendText(bgCtx, task.RoomID, str)
			_ = r.logger.Log("error", logStr, logger.Options{})
			r.matrix.SendToLogRoom(bgCtx, logStr)
			return
		}
	})
	return cronID, err
}
