package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"nozomi/internal/logger"

	"google.golang.org/genai"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/format"
)

func saveQuota() {
	path := filepath.Join(workdir, "data", "search_quota.json")
	// 使用 json.MarshalIndent 可以让生成的 JSON 文件有缩进
	data, _ := json.MarshalIndent(quota, "", "\t")
	err := os.WriteFile(path, data, 0644)
	if err != nil {
		str := "Saving data to search_quota.json fail." + err.Error()
		logger.Log("error", str, logger.Options{})
		sendToLogRoom(str)
	}
}

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

func historyCleanup(history []*genai.Content) []*genai.Content {
	totalLength := getHistoryLen(history)
	for totalLength > botConfig.Client.MaxMemoryLength {
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

func extractMatrixReply(raw string) (string, string) {
	if !strings.HasPrefix(raw, ">") {
		return "", raw
	}

	lines := strings.Split(raw, "\n")
	var quoteLines []string
	var replyLines []string
	inQuote := true

	for _, line := range lines {
		if inQuote {
			if strings.HasPrefix(line, ">") {
				// 使用 TrimLeft 剥离当前行所有前导的 > 和空格
				// 把这种多层嵌套降维成纯文本
				trimmed := strings.TrimLeft(line, "> ")
				quoteLines = append(quoteLines, trimmed)
			} else if strings.TrimSpace(line) == "" {
				// 遇到纯空行，代表 Markdown 引用区正式结束
				inQuote = false
			} else {
				// 容错处理：如果不遵守规范（没打空行直接写回复），强行切断
				inQuote = false
				replyLines = append(replyLines, line)
			}
		} else {
			replyLines = append(replyLines, line)
		}
	}

	return strings.TrimSpace(strings.Join(quoteLines, "\n")), strings.TrimSpace(strings.Join(replyLines, "\n"))
}

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

	if msg.MsgType != event.MsgText && msg.MsgType != event.MsgImage {
		return
	}

	var req string = msg.Body
	// 回复处理
	quoteStr, replyStr := extractMatrixReply(req)
	if quoteStr != "" {
		// Fallback
		req = fmt.Sprintf("(引用回复了：“%s”)\n%s", quoteStr, replyStr)
	} else if msg.RelatesTo != nil && msg.RelatesTo.InReplyTo != nil {
		// MSC2802
		replyToID := msg.RelatesTo.InReplyTo.EventID

		// 主动向服务器拉取被回复的历史消息
		origEvt, err := client.GetEvent(ctx, evt.RoomID, replyToID)
		if err == nil && origEvt != nil {
			if origEvt.Type == event.EventEncrypted && client.Crypto != nil {
				decryptedEvt, decErr := client.Crypto.Decrypt(ctx, origEvt)
				if decErr == nil {
					origEvt = decryptedEvt
				}
			}
			origMsg := origEvt.Content.AsMessage()
			if origMsg != nil {
				origText := origMsg.Body

				// 如果拉取到的历史消息本身也是个引用，只提取它真正说的话，抛弃它当时的引用块
				historyQuote, historyReply := extractMatrixReply(origText)
				if historyQuote != "" {
					origText = historyReply
				}

				req = fmt.Sprintf("(引用回复了：“%s”)\n%s", origText, req)
			}
		}
	}
	mentionPattern := `\[.*?\]\(https://matrix\.to/#/` + regexp.QuoteMeta(string(client.UserID)) + `\)`
	mentionRegex := regexp.MustCompile(mentionPattern)
	req = mentionRegex.ReplaceAllString(req, "")
	req = strings.ReplaceAll(req, "!c ", "")
	req = strings.TrimSpace(req)
	// 文件名嗅探
	var hasRealCaption bool
	if msg.MsgType == event.MsgImage {
		lowerReq := strings.ToLower(req)
		isJustFilename := false
		// 常见的文件名后缀且信息中无空格包含（文件名通常不包含空格）
		if (strings.HasSuffix(lowerReq, ".jpg") || strings.HasSuffix(lowerReq, ".png") ||
			strings.HasSuffix(lowerReq, ".jpeg") || strings.HasSuffix(lowerReq, ".gif") ||
			strings.HasSuffix(lowerReq, ".webp")) && !strings.Contains(req, " ") {
			isJustFilename = true
		}
		if len(req) == 0 || isJustFilename {
			req = "(发送了一张图片)"
		} else {
			hasRealCaption = true
			req = "(发送了一张图片并配文) " + req
		}
	} else {
		if len(req) == 0 {
			req = "(呼叫了你(希))"
		}
	}

	var incomingImgPart *genai.Part
	if msg.MsgType == event.MsgImage {
		var imgData []byte
		var err error
		var mimeType string

		// 1. 如果是加密房间，图片在 msg.File 里
		if msg.File != nil {
			parsedURI, err := msg.File.URL.Parse()
			if err != nil {
				_ = logger.Log("error", "Failed to parse encrypted image URI: "+err.Error(), logger.Options{})
				return
			}
			// 下载加密的乱码包
			imgData, err = client.DownloadBytes(ctx, parsedURI)
			if err != nil {
				_ = logger.Log("error", "Failed to download encrypted image: "+err.Error(), logger.Options{})
				return
			}
			// 使用 Matrix 密钥在内存中原地解密还原图片
			err = msg.File.DecryptInPlace(imgData)
			if err != nil {
				_ = logger.Log("error", "Failed to decrypt image: "+err.Error(), logger.Options{})
				return
			}
			if msg.Info != nil {
				mimeType = msg.Info.MimeType
			} else {
				mimeType = "image/jpeg"
			}
		} else if len(msg.URL) > 0 {
			// 2. 如果是非加密房间，正常从 msg.URL 下载
			parsedURI, parseErr := msg.URL.Parse()
			if parseErr != nil {
				_ = logger.Log("error", "Get image URI error: "+parseErr.Error(), logger.Options{})
				return
			}
			imgData, err = client.DownloadBytes(ctx, parsedURI)
			if err != nil {
				_ = logger.Log("error", "Failed to download Matrix image: "+err.Error(), logger.Options{})
				return
			}
			if msg.Info != nil {
				mimeType = msg.Info.MimeType
			} else {
				mimeType = "image/jpeg"
			}
		}

		// 将解密后的明文图片字节流封装给 Gemini
		if len(imgData) > 0 {
			incomingImgPart = &genai.Part{
				InlineData: &genai.Blob{
					MIMEType: mimeType,
					Data:     imgData,
				},
			}
		} else {
			_ = logger.Log("error", "Image data is empty after processing.", logger.Options{})
			return
		}
	}

	// LoadOrStore 保证了即便多个并发同时到达，也只会初始化出一把唯一的锁
	roomLockObj, _ := roomLocks.LoadOrStore(evt.RoomID.String(), &sync.Mutex{})
	roomLock := roomLockObj.(*sync.Mutex)

	roomLock.Lock()
	defer roomLock.Unlock()

	// 取出当前房间的对话记忆
	var history []*genai.Content
	if val, ok := chatMemory.Load(evt.RoomID.String()); ok {
		history = val.([]*genai.Content)
	}

	// 当前的对话内容
	var currentParts []*genai.Part

	// 房间逻辑
	membersResp, err := client.JoinedMembers(ctx, evt.RoomID)
	if err != nil {
		str := "Failed to get room member list: " + err.Error()
		_ = logger.Log("error", str, logger.Options{})
		sendToLogRoom(str)
		return
	}
	peopleNum := len(membersResp.Joined)
	isGroup := peopleNum > 2
	isMentioned := false
	// 判断是否是群聊
	if isGroup {
		// 群聊逻辑
		if incomingImgPart != nil {
			currentParts = append(currentParts, incomingImgPart)
		}
		if strings.Contains(msg.Body, "!c ") {
			isMentioned = true
		}
		if strings.Contains(msg.Body, string(client.UserID)) {
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
		str := fmt.Sprintf("%s 发言：%s\n", evt.Sender.String(), req)
		currentParts = append(currentParts, genai.Text(str)[0].Parts[0])
	} else {
		// 私信逻辑
		isMentioned = true

		if msg.MsgType == event.MsgImage {
			if incomingImgPart != nil {
				val, _ := privateImageCache.LoadOrStore(evt.RoomID.String(), &ImageCacheItem{})
				cache := val.(*ImageCacheItem)

				cache.Lock()
				// 追加新图之前先判断是否有过期旧图缓存
				if time.Now().After(cache.ExpireTime) && len(cache.Parts) > 0 {
					cache.Parts = nil
				}
				cache.Parts = append(cache.Parts, incomingImgPart)
				cache.ExpireTime = time.Now().Add(5 * time.Minute)
				imgCount := len(cache.Parts)
				cache.Unlock()

				// 如果是一张纯图片，阻断并提示
				if !hasRealCaption {
					_, _ = client.SendText(ctx, evt.RoomID, fmt.Sprintf("收到 %d 张图。请在 5 分钟内补充文字说明。", imgCount))
					return
				}
			} else {
				return // 图片下载失败，直接忽略
			}
		}

		// 走到这里，说明是文字，或者是带真实配文的图片
		val, ok := privateImageCache.Load(evt.RoomID.String())
		if ok {
			cache := val.(*ImageCacheItem)
			cache.Lock()
			if time.Now().Before(cache.ExpireTime) && len(cache.Parts) > 0 {
				currentParts = append(currentParts, cache.Parts...)
			}
			cache.Parts = nil
			cache.Unlock()
			privateImageCache.Delete(evt.RoomID.String())
		}
		currentParts = append(currentParts, genai.Text(req)[0].Parts[0])
	}

	if len(currentParts) == 0 {
		return
	}

	// 防止 Gemini 连续 user 报错：合并同类项
	historyLen := len(history)
	if historyLen > 0 && history[historyLen-1].Role == "user" {
		// 上一句话是人类说的（只有群聊会出现），直接把新的文本 Part 塞进上一个 user 的包裹里
		history[historyLen-1].Parts = append(history[historyLen-1].Parts, currentParts...)
	} else {
		// 上一句话是大模型的，这是一个全新的对话，创建一个新的 user 节点
		userMsg := &genai.Content{
			Role:  "user",
			Parts: currentParts,
		}
		history = append(history, userMsg)
	}
	history = historyCleanup(history)
	chatMemory.Store(evt.RoomID.String(), history)

	if !isMentioned {
		return
	}

	// 深拷贝一份绝对干净的历史记录给大模型，防止指针并发崩溃
	sendHistory := make([]*genai.Content, len(history))
	for i, h := range history {
		partsCopy := make([]*genai.Part, len(h.Parts))
		copy(partsCopy, h.Parts)
		sendHistory[i] = &genai.Content{
			Role:  h.Role,
			Parts: partsCopy,
		}
	}

	go func(evt *event.Event, req string, history []*genai.Content) {
		// 由于是独立协程，不允许再使用外部context
		ctx := context.Background()

		reqConfig := botConfig.Model.Config
		nowMonth := time.Now().Format("2006-01")
		searchMutex.Lock()
		if quota.Month != nowMonth {
			quota.Month = nowMonth
			quota.Count = botConfig.Model.MaxMonthlySearch
			saveQuota()
		}
		if quota.Count <= 0 {
			tempConfig := *reqConfig //解引用拿到浅拷贝
			tempConfig.Tools = nil
			reqConfig = &tempConfig
		}
		searchMutex.Unlock()

		// 调用大模型
		result, costTime, err := Call(history, reqConfig)
		if err != nil {
			str := "用户：" + evt.Sender.String() + "\n"
			str += "房间：" + evt.RoomID.String() + "\n"
			str += "请求：" + req + "\n"
			str += "时间：" + time.UnixMilli(evt.Timestamp).Format("2006-01-02 15:04:05") + "\n"

			errMsg := err.Error()
			isLocalTimeout := errors.Is(err, context.DeadlineExceeded)
			isRemoteTimeout := strings.Contains(errMsg, "DEADLINE_EXCEEDED") || strings.Contains(errMsg, "504")
			if isLocalTimeout || isRemoteTimeout {
				str += fmt.Sprintf("大模型调用超时：%v", costTime)
				_, _ = client.SendText(ctx, evt.RoomID, "Network congestion.Please try again later.")
				_ = logger.Log("error", fmt.Sprintf("Call LLM time out, spent: %v", costTime), logger.Options{})
			} else {
				_, _ = client.SendText(ctx, evt.RoomID, "Sorry, I need rest.")
				_ = logger.Log("error", fmt.Sprintf("Gemini meet a error: %s", err.Error()), logger.Options{})

				str += "错误：" + err.Error()
			}

			sendToLogRoom(str)
			return
		}

		inputToken := result.UsageMetadata.PromptTokenCount
		outputToken := result.UsageMetadata.CandidatesTokenCount
		totalToken := result.UsageMetadata.TotalTokenCount
		thinkToken := totalToken - outputToken - inputToken
		tokenConsume := fmt.Sprintf(
			" | 输入%d 输出%d 总计消耗%d | %v",
			inputToken,
			outputToken,
			totalToken,
			costTime,
		)
		_ = logger.Log("bot", req+tokenConsume, logger.Options{UserID: evt.Sender.String(), RoomID: evt.RoomID.String()})
		tokenMutex.Lock()
		CheckAndResetBilling()
		GlobalTokenUsage.Record(inputToken, outputToken, thinkToken)
		SaveTokenUsage()
		tokenMutex.Unlock()

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

		if len(result.Candidates) > 0 && result.Candidates[0].GroundingMetadata != nil {
			// 只要 GroundingMetadata 不为空，且包含了搜索入口或数据块，就说明大模型悄悄上网了
			meta := result.Candidates[0].GroundingMetadata
			if meta.SearchEntryPoint != nil || len(meta.GroundingChunks) > 0 {
				// fmt.Println("大模型搜索了:", meta.WebSearchQueries)
				searchMutex.Lock()
				quota.Count--
				saveQuota()
				searchMutex.Unlock()
			}
		}

		raw := result.Text()
		raw = strings.TrimSpace(raw)
		re := regexp.MustCompile(`\n{3,}`)
		raw = re.ReplaceAllString(raw, "\n\n")

		if len(result.Candidates) > 0 && result.Candidates[0].Content != nil {
			cleanParts := genai.Text(raw)[0].Parts
			safeModelMsg := &genai.Content{
				Role:  "model",
				Parts: cleanParts,
			}
			if len(safeModelMsg.Parts) > 0 {
				roomLock.Lock()
				if val, ok := chatMemory.Load(evt.RoomID.String()); ok {
					latestHistory := val.([]*genai.Content)
					latestHistory = append(latestHistory, safeModelMsg)
					latestHistory = historyCleanup(latestHistory)
					chatMemory.Store(evt.RoomID.String(), latestHistory)
				}
				roomLock.Unlock()
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
	}(evt, req, sendHistory)
}
