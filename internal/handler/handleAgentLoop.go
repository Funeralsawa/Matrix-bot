package handler

import (
	"context"
	"fmt"
	"nozomi/internal/llm"
	"nozomi/internal/logger"
	"time"

	"google.golang.org/genai"
	"maunium.net/go/mautrix/id"
)

var maxStep = 10

type AgentResponse struct {
	LastResult *llm.GenerateResult
	Usage      *llm.TokenUsage
	TimeCost   time.Duration
	History    []*genai.Content
	Err        error
}

func (r *Router) RunAgentLoop(bgCtx context.Context, safeHistory []*genai.Content, sender id.UserID, rID id.RoomID) *AgentResponse {
	var response = &AgentResponse{
		History: safeHistory,
		Usage:   &llm.TokenUsage{},
	}
	for retry := 0; retry < maxStep; retry++ {

		// 判断联网次数是否耗光
		var dynamicConfig *genai.GenerateContentConfig
		if r.quota.CheckAndGetRemaining() <= 0 {
			dynamicConfig = r.llm.GetConfigWithoutSearch()
		}

		// Call LLM
		res, usage, err := r.llm.Generate(bgCtx, response.History, dynamicConfig)
		if err != nil {
			response.Err = err
			r.logger.Log("error", err.Error(), logger.Options{})
			time.Sleep(1 * time.Second)
			continue
		} else {
			response.LastResult = res
			response.Err = nil
		}

		// 联网搜寻额度抵扣
		if res.UsedSearch {
			// 扣减一次额度
			r.quota.Consume()
		}

		// token 消耗记录
		response.Usage.Input += usage.Input
		response.Usage.Output += usage.Output
		response.Usage.Think += usage.Think

		// 时间记录
		response.TimeCost += res.CostTime

		if res.FunCall != nil && retry == maxStep-2 {
			str := "[System: This is the last step. Please summarize the information you have and give a final response now. DO NOT call any more tools.]"
			response.History = append(response.History, &genai.Content{
				Role:  "model",
				Parts: []*genai.Part{res.FunCall},
			})
			response.History = append(response.History, genai.Text(str)...)
		} else if res.FunCall != nil && retry < maxStep-2 {
			response.History = r.HandleFunCall(bgCtx, rID, sender, response.History, res.FunCall)
			continue
		}
		break
	}
	return response
}

func (r *Router) agentLog(agentResponse *AgentResponse, text string, sender id.UserID, rID id.RoomID) {
	tokenConsumeStr := fmt.Sprintf(
		"%s | input %d output %d total %d | %v",
		text,
		agentResponse.Usage.Input,
		agentResponse.Usage.Output,
		agentResponse.Usage.Think+agentResponse.Usage.Input+agentResponse.Usage.Output,
		agentResponse.TimeCost,
	)
	_ = r.logger.Log("bot", tokenConsumeStr, logger.Options{UserID: sender.String(), RoomID: rID.String()})
}

func (r *Router) billingRecord(bgCtx context.Context, text string, agentResponse *AgentResponse, sender id.UserID, rID id.RoomID, evtTime time.Time) {
	r.billing.Record(agentResponse.Usage.Input, agentResponse.Usage.Output, agentResponse.Usage.Think)
	if r.billing.CheckAlarm(agentResponse.Usage.Input + agentResponse.Usage.Output + agentResponse.Usage.Think) {
		str := fmt.Sprintf(
			"Dosage Alert!\nUser: %v\nroom: %v\nrequest: %s\ntime: %v\ninput %d output %d total %d",
			sender,
			rID,
			text,
			evtTime.Format("2006-01-02 15:04:05"),
			agentResponse.Usage.Input,
			agentResponse.Usage.Output,
			agentResponse.Usage.Think+agentResponse.Usage.Input+agentResponse.Usage.Output,
		)
		errs := r.matrix.SendToLogRoom(bgCtx, str)
		for _, err := range errs {
			r.logger.Log("error", "Sending log to log-room error: "+err.Error(), logger.Options{})
		}
	}
}
