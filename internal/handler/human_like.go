package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"PakuchiBot/internal/bot"
	"PakuchiBot/internal/storage"

	"github.com/sirupsen/logrus"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

type HumanLikeHandler struct {
	bot            *zero.Ctx
	messageHistory map[int64][]Message
	mutex          sync.RWMutex
}

type Message struct {
	UserID       int64
	Content      string
	Timestamp    time.Time
	IsFromBot    bool
	RefMessageID int64
}

type APIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type LLMResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func NewHumanLikeHandler() *HumanLikeHandler {
	return &HumanLikeHandler{
		bot:            zero.GetBot(bot.Config.Bot.SelfID),
		messageHistory: make(map[int64][]Message),
		mutex:          sync.RWMutex{},
	}
}

func RegisterHumanLikeHandler() {
	if !bot.Config.HumanLike.Enabled {
		log.Println("HumanLike function is not enabled")
		return
	}

	handler := NewHumanLikeHandler()
	handler.Register()
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
					go h.generateAndSendReply(ctx)
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
	if ctx.Event.UserID == bot.Config.Bot.SelfID {
		return
	}

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

		var enhancedText string
		if ctx.Event.IsToMe {
			atCodeRegex := regexp.MustCompile(`\[CQ:at,qq=(\d+)(,.*?)?\]`)
			if atCodeRegex.MatchString(msgText) {
				enhancedText = atCodeRegex.ReplaceAllStringFunc(msgText, func(match string) string {
					matches := atCodeRegex.FindStringSubmatch(match)
					if len(matches) >= 2 && matches[1] == strconv.FormatInt(bot.Config.Bot.SelfID, 10) {
						var botName string
						if len(bot.Config.Bot.NickNames) > 0 {
							botName = bot.Config.Bot.NickNames[0]
						} else {
							botName = "机器人"
						}
						return "[提示：用户在这里@了你(" + botName + ")] "
					}
					return match
				})
			} else {
				var botName string
				if len(bot.Config.Bot.NickNames) > 0 {
					botName = bot.Config.Bot.NickNames[0]
				} else {
					botName = "机器人"
				}
				enhancedText = "[提示：用户在这里@了你(" + botName + ")] " + msgText
			}
		} else {
			enhancedText = msgText
		}

		msg := Message{
			UserID:       ctx.Event.UserID,
			Content:      enhancedText,
			Timestamp:    time.Now(),
			IsFromBot:    false,
			RefMessageID: msgID,
		}

		history := h.messageHistory[groupID]
		if len(history) >= 20 {
			history = history[1:]
		}
		h.messageHistory[groupID] = append(history, msg)
	}
}

func (h *HumanLikeHandler) shouldReply(ctx *zero.Ctx) bool {
	if ctx.Event.UserID == bot.Config.Bot.SelfID {
		return false
	}

	if ctx.Event.IsToMe {
		return true
	}

	for _, nickname := range bot.Config.Bot.NickNames {
		if strings.Contains(ctx.Event.RawMessage, nickname) {
			return rand.Float64() < 0.8
		}
	}

	return rand.Float64() < 0.1
}

func (h *HumanLikeHandler) generateAndSendReply(ctx *zero.Ctx) {
	delay := time.Duration(1+rand.Intn(3)) * time.Second
	time.Sleep(delay)

	h.mutex.RLock()
	history, exists := h.messageHistory[ctx.Event.GroupID]
	h.mutex.RUnlock()

	if !exists || len(history) == 0 {
		return
	}

	var apiMessages []APIMessage
	apiMessages = append(apiMessages, APIMessage{
		Role:    "system",
		Content: storage.HumanLikePrompts.ChatSystemPrompt,
	})

	start := 0
	if len(history) > 10 {
		start = len(history) - 10
	}

	historyXML := h.formatHistoryAsXML(history[start:])

	apiMessages = append(apiMessages, APIMessage{
		Role:    "user",
		Content: historyXML,
	})

	reply, err := h.callLLMAPI(apiMessages)
	if err != nil {
		logrus.Errorf("fail to call LLM API: %v", err)
		return
	}

	if strings.Contains(reply, "not_needed") {
		logrus.Debugf("LLM decided not to reply with: %s", reply)
		return
	}

	if strings.Contains(reply, "<content>") && strings.Contains(reply, "</content>") {
		contentStart := strings.Index(reply, "<content>") + len("<content>")
		contentEnd := strings.Index(reply, "</content>")
		if contentStart < contentEnd {
			content := strings.TrimSpace(reply[contentStart:contentEnd])
			if strings.Contains(content, "not_needed") {
				logrus.Debugf("LLM decided not to reply with content: %s", content)
				return
			}
		}
	}

	var msgID int64
	var content string

	if strings.Contains(reply, "<upstream_message>") && strings.Contains(reply, "</upstream_message>") {
		upstreamStart := strings.Index(reply, "<upstream_message>") + len("<upstream_message>")
		upstreamEnd := strings.Index(reply, "</upstream_message>")

		if upstreamStart < upstreamEnd {
			upstreamIDStr := strings.TrimSpace(reply[upstreamStart:upstreamEnd])
			upstreamID, err := strconv.ParseInt(upstreamIDStr, 10, 64)

			if err == nil {
				if strings.Contains(reply, "<content>") && strings.Contains(reply, "</content>") {
					contentStart := strings.Index(reply, "<content>") + len("<content>")
					contentEnd := strings.Index(reply, "</content>")
					if contentStart < contentEnd {
						content = strings.TrimSpace(reply[contentStart:contentEnd])
					} else {
						content = h.cleanReplyContent(reply)
					}
				} else {
					content = h.cleanReplyContent(reply)
				}

				sendMsg := message.Message{}
				sendMsg = append(sendMsg, message.Reply(upstreamID))
				sendMsg = append(sendMsg, message.Text(content))
				msgID = h.bot.SendGroupMessage(ctx.Event.GroupID, sendMsg)
				h.recordBotReply(ctx.Event.GroupID, content, msgID)
				return
			}
		}
	}

	if strings.Contains(reply, "<content>") && strings.Contains(reply, "</content>") {
		contentStart := strings.Index(reply, "<content>") + len("<content>")
		contentEnd := strings.Index(reply, "</content>")
		if contentStart < contentEnd {
			content = strings.TrimSpace(reply[contentStart:contentEnd])
			msgID = h.bot.SendGroupMessage(ctx.Event.GroupID, message.Text(content))
			h.recordBotReply(ctx.Event.GroupID, content, msgID)
			return
		}
	}

	cleanReply := h.cleanReplyContent(reply)
	msgID = h.bot.SendGroupMessage(ctx.Event.GroupID, message.Text(cleanReply))
	h.recordBotReply(ctx.Event.GroupID, cleanReply, msgID)
}

func (h *HumanLikeHandler) cleanReplyContent(reply string) string {
	if strings.Contains(reply, "<content>") && strings.Contains(reply, "</content>") {
		contentStart := strings.Index(reply, "<content>") + len("<content>")
		contentEnd := strings.Index(reply, "</content>")
		if contentStart < contentEnd {
			return strings.TrimSpace(reply[contentStart:contentEnd])
		}
	}

	tagsToRemove := []string{
		"<upstream_message>", "</upstream_message>",
		"<reply>", "</reply>",
		"<msg>", "</msg>",
		"<history>", "</history>",
	}

	result := reply
	for _, tag := range tagsToRemove {
		result = strings.ReplaceAll(result, tag, "")
	}

	return strings.TrimSpace(result)
}

func (h *HumanLikeHandler) formatHistoryAsXML(history []Message) string {
	historyXML := "<history>\n"
	for _, msg := range history {
		historyXML += "<msg>\n"

		senderName := ""
		if msg.IsFromBot {
			if len(bot.Config.Bot.NickNames) > 0 {
				senderName = bot.Config.Bot.NickNames[0]
			} else {
				senderName = "Bot"
			}
			historyXML += "<is_you>true</is_you>\n"
		} else {
			senderName = fmt.Sprintf("User%d", msg.UserID)
		}

		historyXML += "<sender>\n" + h.cleanXMLText(senderName) + "\n</sender>\n"

		content := msg.Content
		if msg.IsFromBot {
			content = "[提示: 这是你自己发送的消息] " + content
		}

		historyXML += "<content>\n" + h.cleanXMLText(content) + "\n</content>\n"
		historyXML += "<id>\n" + fmt.Sprintf("%d", msg.RefMessageID) + "\n</id>\n"
		historyXML += "</msg>\n"
	}
	historyXML += "</history>"

	return historyXML
}

func (h *HumanLikeHandler) cleanXMLText(text string) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	text = strings.ReplaceAll(text, "\"", "&quot;")
	text = strings.ReplaceAll(text, "'", "&apos;")
	return text
}

func (h *HumanLikeHandler) callLLMAPI(apiMessages []APIMessage) (string, error) {
	requestBody := map[string]interface{}{
		"model":       bot.Config.HumanLike.LLM.Model,
		"messages":    apiMessages,
		"temperature": bot.Config.HumanLike.LLM.Temperature,
		"max_tokens":  bot.Config.HumanLike.LLM.MaxTokens,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("fail to create request: %v", err)
	}

	req, err := http.NewRequest("POST", bot.Config.HumanLike.LLM.BaseURL+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("fail to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bot.Config.HumanLike.LLM.APIKey)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fail to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API returned error status code: %d, response: %s", resp.StatusCode, string(body))
	}

	var llmResp LLMResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("fail to read response: %v", err)
	}

	if err := json.Unmarshal(body, &llmResp); err != nil {
		return "", fmt.Errorf("fail to parse response: %v", err)
	}

	if len(llmResp.Choices) == 0 {
		return "", fmt.Errorf("API response has no valid content")
	}

	return llmResp.Choices[0].Message.Content, nil
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
