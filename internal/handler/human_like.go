package handler

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"PakuchiBot/internal/bot"

	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

type HumanLikeHandler struct {
	bot            *zero.Ctx
	messageHistory map[int64][]Message
	lastActiveTime map[int64]time.Time
	mutex          sync.RWMutex
	stopChan       chan struct{}
}

type Message struct {
	UserID       int64
	Content      string
	Timestamp    time.Time
	IsFromBot    bool
	RefMessageID int64
}

type LLMResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type LLMStreamResponse struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

type LLMRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens"`
}

type APIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func NewHumanLikeHandler() *HumanLikeHandler {
	return &HumanLikeHandler{
		bot:            zero.GetBot(bot.Config.Bot.SelfID),
		messageHistory: make(map[int64][]Message),
		lastActiveTime: make(map[int64]time.Time),
		mutex:          sync.RWMutex{},
		stopChan:       make(chan struct{}),
	}
}

func RegisterHumanLikeHandler() {
	if !bot.Config.HumanLike.Enabled {
		log.Println("HumanLike function is not enabled")
		return
	}

	handler := NewHumanLikeHandler()
	handler.Register()

	go handler.StartProactiveChecker()
}

func (h *HumanLikeHandler) Register() {
	zero.OnMessage().SetBlock(false).
		Handle(func(ctx *zero.Ctx) {
			if ctx.Event.UserID == bot.Config.Bot.SelfID {
				return
			}

			if ctx.Event.GroupID != 0 {
				if !h.isGroupInWhitelist(ctx.Event.GroupID) {
					return
				}

				h.recordMessage(ctx)

				if h.shouldReply(ctx) {
					go h.replyWithDelay(ctx)
				}
			}
		})
}

func (h *HumanLikeHandler) isGroupInWhitelist(groupID int64) bool {
	if !bot.Config.HumanLike.Behavior.EnableGroupWhitelist {
		return true
	}

	if len(bot.Config.HumanLike.Behavior.GroupWhitelist) == 0 {
		return true
	}

	for _, id := range bot.Config.HumanLike.Behavior.GroupWhitelist {
		if id == groupID {
			return true
		}
	}
	return false
}

func (h *HumanLikeHandler) recordMessage(ctx *zero.Ctx) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	groupID := ctx.Event.GroupID
	if _, exists := h.messageHistory[groupID]; !exists {
		h.messageHistory[groupID] = make([]Message, 0)
	}

	msgText := ctx.Event.RawMessage
	if len(msgText) > 0 {
		var msgID int64
		if id, ok := ctx.Event.MessageID.(int64); ok {
			msgID = id
		} else {
			msgID = time.Now().UnixNano()
		}

		msg := Message{
			UserID:       ctx.Event.UserID,
			Content:      msgText,
			Timestamp:    time.Now(),
			IsFromBot:    false,
			RefMessageID: msgID,
		}

		history := h.messageHistory[groupID]
		if len(history) >= 20 {
			history = history[1:]
		}
		h.messageHistory[groupID] = append(history, msg)

		h.lastActiveTime[groupID] = time.Now()
	}
}

func (h *HumanLikeHandler) shouldReply(ctx *zero.Ctx) bool {
	// reply if the message is @ the bot
	if ctx.Event.IsToMe {
		return true
	}

	// check if the message is replying to the bot's last message
	isReplyingToBot := h.isReplyingToBot(ctx)
	if isReplyingToBot {
		return true
	}

	// if the message contains the bot's nickname, reply with a probability of 80%
	for _, nickname := range bot.Config.Bot.NickNames {
		if strings.Contains(ctx.Event.RawMessage, nickname) {
			return rand.Float64() < 0.8
		}
	}

	// if the user has a recent conversation with the bot, reply with a probability of 40%
	if h.hasRecentConversation(ctx.Event.GroupID, ctx.Event.UserID) {
		return rand.Float64() < 0.4
	}

	// default reply probability is 10%
	return rand.Float64() < 0.1
}

func (h *HumanLikeHandler) isReplyingToBot(ctx *zero.Ctx) bool {
	h.mutex.RLock()
	history, exists := h.messageHistory[ctx.Event.GroupID]
	h.mutex.RUnlock()

	if !exists || len(history) == 0 {
		return false
	}

	lastMsg := history[len(history)-1]
	if lastMsg.IsFromBot && time.Since(lastMsg.Timestamp) < 2*time.Minute {
		return true
	}

	const recentWindow = 5
	start := len(history) - recentWindow
	if start < 0 {
		start = 0
	}

	userMessageCount := 0
	botMessageCount := 0
	lastWasBot := false
	hasAlternatingPattern := false

	for i := start; i < len(history); i++ {
		msg := history[i]

		if msg.UserID == ctx.Event.UserID {
			userMessageCount++
			if lastWasBot {
				hasAlternatingPattern = true
			}
			lastWasBot = false
		} else if msg.IsFromBot {
			botMessageCount++
			if !lastWasBot && userMessageCount > 0 {
				hasAlternatingPattern = true
			}
			lastWasBot = true
		}
	}

	if userMessageCount >= 1 && botMessageCount >= 1 && hasAlternatingPattern &&
		time.Since(history[len(history)-1].Timestamp) < 3*time.Minute {
		return true
	}

	return false
}

func (h *HumanLikeHandler) hasRecentConversation(groupID int64, userID int64) bool {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	history, exists := h.messageHistory[groupID]
	if !exists || len(history) < 3 {
		return false
	}

	start := len(history) - 5
	if start < 0 {
		start = 0
	}

	botRepliedToUser := false
	userMessageCount := 0

	for i := start; i < len(history); i++ {
		msg := history[i]

		if msg.UserID == userID {
			userMessageCount++
		}

		if msg.IsFromBot && i > 0 && history[i-1].UserID == userID {
			botRepliedToUser = true
		}
	}

	return userMessageCount >= 2 && botRepliedToUser
}

func (h *HumanLikeHandler) replyWithDelay(ctx *zero.Ctx) {
	err := h.generateAndProcessReply(ctx.Event.GroupID, func(msg string, isLast bool) {
		sendMsg := message.Text(msg)
		msgID := h.bot.SendGroupMessage(ctx.Event.GroupID, sendMsg)

		h.recordBotReply(ctx.Event.GroupID, msg, msgID)

		if !isLast {
			time.Sleep(time.Duration(rand.Intn(3)+1) * time.Second)
		}
	})

	if err != nil {
		log.Printf("generate reply failed: %v", err)
	}
}

func (h *HumanLikeHandler) streamLLMRequest(
	apiMessages []APIMessage,
	messageHandler func(string, bool),
	errorPrefix string,
) error {
	requestBody := map[string]interface{}{
		"model":       bot.Config.HumanLike.LLM.Model,
		"messages":    apiMessages,
		"temperature": bot.Config.HumanLike.LLM.Temperature,
		"max_tokens":  bot.Config.HumanLike.LLM.MaxTokens,
		"stream":      true,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("%s convert JSON failed: %v", errorPrefix, err)
	}

	req, err := http.NewRequest("POST", bot.Config.HumanLike.LLM.BaseURL+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("%s create request failed: %v", errorPrefix, err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bot.Config.HumanLike.LLM.APIKey)
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s send request failed: %v", errorPrefix, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s API returned error status code: %d, response: %s", errorPrefix, resp.StatusCode, string(body))
	}

	return h.processStreamResponse(resp.Body, messageHandler, errorPrefix)
}

func (h *HumanLikeHandler) processStreamResponse(
	responseBody io.ReadCloser,
	messageHandler func(string, bool),
	errorPrefix string,
) error {
	reader := bufio.NewReader(responseBody)
	var messageBuffer strings.Builder
	var messages []string
	var isCollecting bool = true
	var hasReachedEnd bool = false

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				hasReachedEnd = true
				break
			}
			return fmt.Errorf("%s read response stream failed: %v", errorPrefix, err)
		}

		line = strings.TrimSpace(line)
		if line == "" || line == "data: [DONE]" {
			if line == "data: [DONE]" {
				hasReachedEnd = true
			}
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			line = strings.TrimPrefix(line, "data: ")
		}

		var streamResp LLMStreamResponse
		if err := json.Unmarshal([]byte(line), &streamResp); err != nil {
			log.Printf("%s parse stream response failed: %v, line: %s", errorPrefix, err, line)
			continue
		}

		if len(streamResp.Choices) > 0 {
			content := streamResp.Choices[0].Delta.Content
			if content != "" {
				messageBuffer.WriteString(content)

				bufferContent := messageBuffer.String()

				if strings.Contains(bufferContent, "<msg_break>") {
					parts := strings.Split(bufferContent, "<msg_break>")
					if len(parts) > 1 {
						for i := 0; i < len(parts)-1; i++ {
							part := strings.TrimSpace(parts[i])
							if part != "" {
								messages = append(messages, part)

								if isCollecting {
									h.simulateTypingDelay(part)
									messageHandler(part, false)
								}
							}
						}

						messageBuffer.Reset()
						messageBuffer.WriteString(strings.TrimSpace(parts[len(parts)-1]))
					}
				} else if strings.Contains(bufferContent, "\n\n") {
					parts := strings.Split(bufferContent, "\n\n")
					if len(parts) > 1 {
						for i := 0; i < len(parts)-1; i++ {
							part := strings.TrimSpace(parts[i])
							if part != "" {
								messages = append(messages, part)

								if isCollecting {
									h.simulateTypingDelay(part)
									messageHandler(part, false)
								}
							}
						}

						messageBuffer.Reset()
						messageBuffer.WriteString(strings.TrimSpace(parts[len(parts)-1]))
					}
				}
			}

			if streamResp.Choices[0].FinishReason != "" {
				hasReachedEnd = true
			}
		}

		bufferContent := messageBuffer.String()

		partialBreakMatch := h.hasPartialBreakMatch(bufferContent)
		partialNewlineMatch := h.hasPartialNewlineMatch(bufferContent)

		// process the current buffer if the buffer content is long enough (and not a partial match of the separator) or has reached the end
		if (messageBuffer.Len() > 100 && !strings.Contains(bufferContent, "<msg_break>") && !strings.Contains(bufferContent, "\n\n") && !partialBreakMatch && !partialNewlineMatch) || hasReachedEnd {
			currentMessage := strings.TrimSpace(bufferContent)
			if currentMessage != "" {
				if isCollecting {
					h.simulateTypingDelay(currentMessage)
					// if it's the last message, mark isLast as true
					messageHandler(currentMessage, hasReachedEnd && len(messages) == 0)
					messages = append(messages, currentMessage)
				}
				messageBuffer.Reset()
			}

			if hasReachedEnd {
				break
			}
		}
	}

	return nil
}

func (h *HumanLikeHandler) hasPartialBreakMatch(content string) bool {
	for i := 1; i < len("<msg_break>") && i < len(content); i++ {
		suffix := content[len(content)-i:]
		prefix := "<msg_break>"[:i]
		if suffix == prefix {
			return true
		}
	}
	return false
}

func (h *HumanLikeHandler) hasPartialNewlineMatch(content string) bool {
	if len(content) < 2 {
		return false
	}

	if content[len(content)-1] == '\n' {
		return true
	}
	return false
}

func (h *HumanLikeHandler) generateAndProcessReply(groupID int64, messageHandler func(string, bool)) error {
	h.mutex.RLock()
	history, exists := h.messageHistory[groupID]
	h.mutex.RUnlock()

	if !exists || len(history) == 0 {
		return fmt.Errorf("no history messages")
	}

	var apiMessages []APIMessage

	systemPrompt := "你是一个群聊中的普通人类成员，需要像普通人一样自然地聊天。遵循以下规则：\n" +
		"1. 不要使用机器人或AI助手的口吻\n" +
		"2. 不要过于正式或客套\n" +
		"3. 可以使用些许表情符号和网络用语\n" +
		"4. 回复要简短自然，不需要很全面，可以有自己的态度和观点\n" +
		"5. 不要表明你是AI或机器人\n" +
		"6. 不要过度解释\n" +
		"7. 如果你想模拟人类发送多条消息的习惯，请使用 <msg_break> 作为分隔符。例如：'这个问题很有意思<msg_break>我觉得应该这样解决'\n" +
		"8. 根据内容的自然停顿和逻辑分隔，适当使用分隔符，像人类一样把一个长回复分成多条消息发送\n" +
		"9. 你可以适当使用 emoji 但切记不要过度使用\n" +
		"10. 有的时候你可以直接回复一个符号，这样会更像人类，例如当遇到一些很离谱的问题时，你可以回复 '？' 或者当遇到一些让你很无语的话时，你可以回复 '。' 等符号，以此类推\n"

	apiMessages = append(apiMessages, APIMessage{
		Role:    "system",
		Content: systemPrompt,
	})

	// add the recent history messages, at most 10
	start := 0
	if len(history) > 10 {
		start = len(history) - 10
	}

	for _, msg := range history[start:] {
		role := "user"
		if msg.IsFromBot {
			role = "assistant"
		}
		apiMessages = append(apiMessages, APIMessage{
			Role:    role,
			Content: msg.Content,
		})
	}

	return h.streamLLMRequest(apiMessages, messageHandler, "reply")
}

func (h *HumanLikeHandler) simulateTypingDelay(text string) {
	// calculate the typing time
	// assume the human typing speed is between min_typing_speed and max_typing_speed characters per second
	minSpeed := bot.Config.HumanLike.Behavior.MinTypingSpeed
	maxSpeed := bot.Config.HumanLike.Behavior.MaxTypingSpeed

	if minSpeed <= 0 {
		minSpeed = 3
	}
	if maxSpeed <= 0 {
		maxSpeed = 8
	}

	speed := rand.Intn(maxSpeed-minSpeed+1) + minSpeed
	length := len([]rune(text))

	typingTime := length * 1000 / speed

	randomFactor := rand.Float64()*0.3 + 0.85
	typingTime = int(float64(typingTime) * randomFactor)

	if typingTime > 15000 {
		typingTime = 15000
	}

	time.Sleep(time.Duration(typingTime) * time.Millisecond)
}

func (h *HumanLikeHandler) recordBotReply(groupID int64, content string, msgID int64) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	if _, exists := h.messageHistory[groupID]; !exists {
		h.messageHistory[groupID] = make([]Message, 0)
	}

	msg := Message{
		UserID:       bot.Config.Bot.SelfID,
		Content:      content,
		Timestamp:    time.Now(),
		IsFromBot:    true,
		RefMessageID: msgID,
	}

	history := h.messageHistory[groupID]
	if len(history) >= 20 {
		history = history[1:]
	}
	h.messageHistory[groupID] = append(history, msg)
}

func (h *HumanLikeHandler) StartProactiveChecker() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.checkProactiveSpeaking()
		case <-h.stopChan:
			return
		}
	}
}

func (h *HumanLikeHandler) Stop() {
	close(h.stopChan)
}

func (h *HumanLikeHandler) checkProactiveSpeaking() {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	for groupID, lastActive := range h.lastActiveTime {
		if !h.isGroupInWhitelist(groupID) {
			continue
		}

		timeElapsed := time.Since(lastActive)

		if timeElapsed < 3*time.Minute {
			if rand.Float64() < 0.2 {
				go h.checkShouldProactivelySpeakByLLM(groupID)
			}
		} else if timeElapsed < 10*time.Minute {
			if rand.Float64() < 0.1 {
				go h.checkShouldProactivelySpeakByLLM(groupID)
			}
		} else if timeElapsed < 30*time.Minute {
			if rand.Float64() < 0.05 {
				go h.checkShouldProactivelySpeakByLLM(groupID)
			}
		}
	}
}

func (h *HumanLikeHandler) checkShouldProactivelySpeakByLLM(groupID int64) {
	h.mutex.RLock()
	history, exists := h.messageHistory[groupID]
	h.mutex.RUnlock()

	if !exists || len(history) == 0 {
		return
	}

	var apiMessages []APIMessage

	systemPrompt := "你是群聊中的一个普通成员。请判断是否需要在当前对话中主动参与。请考虑以下因素：\n" +
		"1. 当前话题是否需要你的参与\n" +
		"2. 是否有人在寻求回应\n" +
		"3. 对话是否陷入沉默需要推动\n" +
		"4. 有没有有价值的内容可以分享\n" +
		"5. 不要过多发言，不要总是参与话题，不要总是发言，太吵了\n" +
		"请只回答'yes'或'no'，不要解释原因。"

	apiMessages = append(apiMessages, APIMessage{
		Role:    "system",
		Content: systemPrompt,
	})

	start := 0
	if len(history) > 10 {
		start = len(history) - 10
	}

	contextPrompt := "以下是群聊中最近的一些消息："
	for _, msg := range history[start:] {
		name := "其他成员"
		if msg.IsFromBot {
			name = "你"
		}
		contextPrompt += fmt.Sprintf("\n%s: %s", name, msg.Content)
	}

	apiMessages = append(apiMessages, APIMessage{
		Role:    "user",
		Content: contextPrompt + "\n\n根据以上对话，你是否需要主动参与对话？只回答'yes'或'no'：",
	})

	var responseBuffer bytes.Buffer

	err := h.streamLLMRequest(apiMessages, func(msg string, isLast bool) {
		responseBuffer.WriteString(msg)
	}, "proactive decision")

	if err != nil {
		log.Printf("failed to decide if should speak: %v", err)
		return
	}

	response := strings.ToLower(strings.TrimSpace(responseBuffer.String()))
	if strings.Contains(response, "yes") {
		go h.sendProactiveMessage(groupID)
	}
}

func (h *HumanLikeHandler) sendProactiveMessage(groupID int64) {
	err := h.generateAndProcessProactiveMessage(groupID, func(msg string, isLast bool) {
		sendMsg := message.Text(msg)
		msgID := h.bot.SendGroupMessage(groupID, sendMsg)

		h.recordBotReply(groupID, msg, msgID)

		if !isLast {
			time.Sleep(time.Duration(rand.Intn(3)+1) * time.Second)
		}
	})

	if err != nil {
		log.Printf("proactive speaking failed: %v", err)
	}

	h.mutex.Lock()
	h.lastActiveTime[groupID] = time.Now()
	h.mutex.Unlock()
}

func (h *HumanLikeHandler) generateAndProcessProactiveMessage(groupID int64, messageHandler func(string, bool)) error {
	var apiMessages []APIMessage

	systemPrompt := "你是群聊中的一个普通成员，现在你想主动发起一个话题或参与当前的讨论。请像一个普通人一样自然地发言，可以分享观点、提问、或讨论当前热点话题。不要表明你是AI或机器人，不要过度解释，保持随意自然的语气。\n" +
		"如果你想模拟人类发送多条消息的习惯，请使用 <msg_break> 作为分隔符。例如：'这个游戏最近很火啊<msg_break>有人玩过吗？'"

	apiMessages = append(apiMessages, APIMessage{
		Role:    "system",
		Content: systemPrompt,
	})

	h.mutex.RLock()
	history, exists := h.messageHistory[groupID]
	h.mutex.RUnlock()

	if exists && len(history) > 0 {
		start := 0
		if len(history) > 5 {
			start = len(history) - 5
		}

		contextPrompt := "以下是群聊中最近的一些消息："
		for _, msg := range history[start:] {
			name := "其他成员"
			if msg.IsFromBot {
				name = "你"
			}
			contextPrompt += fmt.Sprintf("\n%s: %s", name, msg.Content)
		}

		apiMessages = append(apiMessages, APIMessage{
			Role:    "user",
			Content: contextPrompt + "\n\n请根据以上上下文，或者如果没有相关上下文，请发起一个新话题。请像普通人一样自然地发言：",
		})
	} else {
		apiMessages = append(apiMessages, APIMessage{
			Role:    "user",
			Content: "请发起一个新的聊天话题，比如分享一个观点、提问或讨论近期的热门话题。保持自然的语气，简短随意：",
		})
	}

	return h.streamLLMRequest(apiMessages, messageHandler, "proactive speaking")
}
