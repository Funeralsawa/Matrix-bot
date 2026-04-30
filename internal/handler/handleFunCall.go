package handler

import (
	"context"
	"fmt"
	"nozomi/internal/logger"
	"nozomi/tools"
	"time"

	"google.golang.org/genai"
	"maunium.net/go/mautrix/id"
)

func (r *Router) HandleFunCall(ctx context.Context, rID id.RoomID, sender id.UserID, history []*genai.Content, fc *genai.Part) []*genai.Content {
	var toolResponseContent string
	var dict map[string]string

	r.logger.Log("info", "LLM called tool: "+fc.FunctionCall.Name, logger.Options{})

	switch fc.FunctionCall.Name {
	case "execute_terminal":
		if !r.checkSenderHasFunCallPower(sender) {
			toolResponseContent = "[Error: The sender don't have enough power to call this tool.]"
			break
		}
		if cmd, ok := fc.FunctionCall.Args["command"].(string); ok {
			r.logger.Log("info", ">_ Execute Command: "+cmd, logger.Options{})

			r.matrix.SendText(ctx, rID, ">_ Execute Command: "+cmd)

			dict = tools.TryExecuteTerminal(cmd, rID, sender)

			toolResponseContent = dict["content"]

			if dict["result"] == "dangerous" {
				str := fmt.Sprintf("**Dangerous command detected**: \n```\n%s\n```\nUsing `/YES [task_id]` or `/NO [task_id]` to determine whether to execute the command.\n`task_id`: `%s`", cmd, dict["task_id"])
				task := tools.CommandTask{RoomID: rID, SenderID: sender, TaskID: dict["task_id"]}
				waitChan := make(chan bool, 1)
				r.pendingApprovals.Store(task, waitChan)
				r.matrix.SendMarkdownWithMath(ctx, rID, str)
				var approved bool
				select {
				case approved = <-waitChan:
					if approved {
						dict = tools.ExecuteTerminal(cmd)
						toolResponseContent = dict["content"]
					} else {
						toolResponseContent = "[Error: User refused authorization]"
					}
				case <-time.After(3 * time.Minute): // 设定一个超时时间，防止协程永久泄漏
					approved = false
					r.matrix.SendText(ctx, rID, "Authorization expired and has been automatically cancelled: "+dict["task_id"])
					toolResponseContent = "[Error: Authorization expired and has been automatically cancelled.]"
				}
				r.pendingApprovals.Delete(task)
				close(waitChan)
			}
		} else {
			toolResponseContent = "[Error: Invalid command argument]"
		}
	case "add_cron_job":
		if !r.checkSenderHasFunCallPower(sender) {
			toolResponseContent = "[Error: The sender don't have enough power to call this tool.]"
			break
		}
		r.matrix.SendText(ctx, rID, "⏰ Adding cron job...")
		cronExpr, _ := fc.FunctionCall.Args["cron_expression"].(string)
		taskPrompt, _ := fc.FunctionCall.Args["task_prompt"].(string)
		cronTask := tools.AddCronJob(rID, sender, cronExpr, taskPrompt)
		// 注入 Cron 引擎
		cronID, err := r.HandleCronJobRegister(cronTask)
		if err != nil {
			toolResponseContent = "[Error: Invalid argument: ]" + err.Error()
			break
		}
		tools.CronID.Store(cronID, cronTask)
		err = r.WriteCronToFile(ctx, cronTask)
		if err != nil {
			toolResponseContent = fmt.Sprintf("[System: Unable to persist cron job.Err: %v]", err)
			tools.RemoveCronJobWithEntryID(cronID)
			break
		}
		toolResponseContent = "[System: Adding cron job sucessfully.]"
		r.matrix.SendText(ctx, rID, fmt.Sprintf("✅ Sucessfully added cron job: %d", cronID))
	case "list_cron_job":
		if !r.checkSenderHasFunCallPower(sender) {
			toolResponseContent = "[Error: The sender don't have enough power to call this tool.]"
			break
		}
		r.matrix.SendText(ctx, rID, "📖 Listing cron job...")
		uuid := ""
		if val, ok := fc.FunctionCall.Args["uuid"].(string); ok {
			uuid = val
		}
		toolResponseContent = tools.ListCronJob(uuid)
	case "remove_cron_job":
		if !r.checkSenderHasFunCallPower(sender) {
			toolResponseContent = "[Error: The sender don't have enough power to call this tool.]"
			break
		}
		r.matrix.SendText(ctx, rID, "(●'◡'●) Removing cron job...")
		if uuid, ok := fc.FunctionCall.Args["uuid"].(string); ok {
			tools.RemoveCronJobWithUUID(uuid)
			err := r.DelCronFromFile(uuid)
			if err != nil {
				toolResponseContent = fmt.Sprintf("[System: The cron task in memory has been cleared, but the persistent copy deletion failed.]")
				break
			}
			toolResponseContent = fmt.Sprintf("[System: Removing cron job %s sucessfully.]", uuid)
			r.matrix.SendText(ctx, rID, "(*/ω＼*) Remove cron job sucessfully.")
		} else {
			toolResponseContent = fmt.Sprintf("[System: Unable to find cron job %s.]", uuid)
		}
	default:
		toolResponseContent = "[Error: Unknown tool]"
	}

	history = append(history, &genai.Content{
		Role:  "model",
		Parts: []*genai.Part{fc},
	})

	history = append(history, &genai.Content{
		Role: "user",
		Parts: []*genai.Part{
			{
				FunctionResponse: &genai.FunctionResponse{
					ID:       fc.FunctionCall.ID,
					Name:     fc.FunctionCall.Name,
					Response: map[string]any{"result": toolResponseContent},
				},
			},
		},
	})
	return history
}
