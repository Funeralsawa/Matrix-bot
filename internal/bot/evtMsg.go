package bot

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"nozomi/internal/logger"

	"google.golang.org/genai"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/format"
)

func evtMsg(ctx context.Context, evt *event.Event) {
	if evt.Timestamp < bootTimeUnixmilli {
		return
	}

	msg := evt.Content.AsMessage()
	if msg == nil {
		return
	}

	if evt.Sender == client.UserID {
		return
	}

	if msg.MsgType == event.MsgText {
		membersResp, err := client.JoinedMembers(ctx, evt.RoomID)
		if err != nil {
			_ = logger.Log("error", " Failed to get room member list: "+err.Error(), logger.Options{})
			return
		}
		peopleNum := len(membersResp.Joined)
		isMentioned := false
		if peopleNum > 2 {
			if strings.HasPrefix(msg.Body, "!c ") {
				isMentioned = true
			}
			if strings.HasPrefix(msg.Body, string(client.UserID)) {
				isMentioned = true
			}
			if msg.Mentions != nil && len(msg.Mentions.UserIDs) > 0 {
				for _, uid := range msg.Mentions.UserIDs {
					if uid == client.UserID {
						isMentioned = true
						break
					}
				}
			}
			if !isMentioned {
				return
			}
		}

		var req string = msg.Body
		mentionPattern := `\[.*?\]\(https://matrix\.to/#/` + regexp.QuoteMeta(string(client.UserID)) + `\)`
		mentionRegex := regexp.MustCompile(mentionPattern)
		req = mentionRegex.ReplaceAllString(req, "")
		req = strings.ReplaceAll(req, string(client.UserID), "")
		req = strings.ReplaceAll(req, "@[希]", "")
		req = strings.ReplaceAll(req, "@希", "")
		req = strings.ReplaceAll(req, "!c ", "")
		req = strings.TrimSpace(req)
		if len(req) > 0 && req[0] == ':' {
			return
		}

		var history []*genai.Content
		if val, ok := chatMemory.Load(evt.RoomID.String()); ok {
			history = val.([]*genai.Content)
		}

		userMsg := genai.Text(req)[0]
		userMsg.Role = "user"
		history = append(history, userMsg)

		if len(history) > botConfig.Client.MaxMemoryLength {
			history = history[len(history)-botConfig.Client.MaxMemoryLength:]
			for history[0].Role == "model" {
				history = history[1:]
			}
		}

		result, err := Call(history)
		if err != nil {
			_, _ = client.SendText(ctx, evt.RoomID, "Sorry, I need rest.")
			_ = logger.Log("error", fmt.Sprintf("Gemini meet a error: %s", err.Error()), logger.Options{})

			str := "用户：" + evt.Sender.String() + "\n"
			str += "房间：" + evt.RoomID.String() + "\n"
			str += "请求：" + req + "\n"
			str += "时间：" + time.UnixMilli(evt.Timestamp).Format("2006-01-02 15:04:05") + "\n"
			str += "错误：" + err.Error()
			sendToLogRoom(str)

			if len(history) > 0 {
				history = history[:len(history)-1]
				chatMemory.Store(evt.RoomID.String(), history)
			}
			return
		}

		tokenConsume := fmt.Sprintf(
			" | 输入%d 输出%d 总计消耗%d",
			result.UsageMetadata.PromptTokenCount,
			result.UsageMetadata.CandidatesTokenCount,
			result.UsageMetadata.TotalTokenCount,
		)
		_ = logger.Log("bot", req+tokenConsume, logger.Options{UserID: evt.Sender.String(), RoomID: evt.RoomID.String()})

		if result.UsageMetadata.TotalTokenCount >= botConfig.Model.AlargmTokenCount {
			tokenConsume = strings.TrimPrefix(tokenConsume, " | ")
			str := "用量警报！\n"
			str += "用户：" + evt.Sender.String() + "\n"
			str += "房间：" + evt.RoomID.String() + "\n"
			str += "请求：" + req + "\n"
			str += "时间: " + time.UnixMilli(evt.Timestamp).Format("2006-01-02 15:04:05") + "\n"
			str += "Token 账单单次达到警报值！\n"
			str += tokenConsume
			sendToLogRoom(str)
		}

		raw := result.Text()
		raw = strings.TrimSpace(raw)
		re := regexp.MustCompile(`\n{3,}`)
		raw = re.ReplaceAllString(raw, "\n\n")

		if len(result.Candidates) > 0 && result.Candidates[0].Content != nil {
			safeModelMsg := result.Candidates[0].Content
			safeModelMsg.Role = "model"

			if len(safeModelMsg.Parts) > 0 {
				history = append(history, safeModelMsg)
				chatMemory.Store(evt.RoomID.String(), history)
			} else {
				str := "The model return null value and has been inhibit.Question: " + req
				_ = logger.Log("info", str, logger.Options{})
				_, _ = client.SendText(ctx, evt.RoomID, "Sorry, I wan't to answer this question.")

				str = "模型返回空警报！\n"
				str += "用户：" + evt.Sender.String() + "\n"
				str += "房间：" + evt.RoomID.String() + "\n"
				str += "请求：" + req + "\n"
				str += "时间: " + time.UnixMilli(evt.Timestamp).Format("2006-01-02 15:04:05")
				sendToLogRoom(str)
				return
			}
		}

		richMsg := format.RenderMarkdown(raw, true, false)
		parts := strings.Split(richMsg.FormattedBody, "<pre>")
		for i := range parts {
			if i == 0 {
				parts[i] = strings.ReplaceAll(parts[i], "\n", "")
			} else {
				subParts := strings.SplitN(parts[i], "</pre>", 2)
				if len(subParts) == 2 {
					subParts[1] = strings.ReplaceAll(subParts[1], "\n", "")
					parts[i] = subParts[0] + "</pre>" + subParts[1]
				}
			}
		}
		richMsg.FormattedBody = strings.Join(parts, "<pre>")

		_, err = client.SendMessageEvent(ctx, evt.RoomID, event.EventMessage, &richMsg)
		if err != nil {
			_ = logger.Log("error", "Failed to send matrix rich message: "+err.Error(), logger.Options{})
			str := "Failed to send matrix rich message.\n"
			str += "用户：" + evt.Sender.String() + "\n"
			str += "房间：" + evt.RoomID.String() + "\n"
			str += "请求：" + req + "\n"
			str += "时间：" + time.UnixMilli(evt.Timestamp).Format("2006-01-02 15:04:05")
			sendToLogRoom(str)
		}
	}
}
