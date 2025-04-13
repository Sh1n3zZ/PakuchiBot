//go:build !test

package handler

import (
	"bufio"
	_ "embed"
	"fmt"
	"hash/fnv"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"PakuchiBot/internal/storage"

	zero "github.com/wdvxdr1123/ZeroBot"
)

//go:embed luck_messages.txt
var luckMessagesData string

// 存储解析后的消息模板
var luckMessages = make(map[string][]string)

// 颜文字列表（我觉得这太蛋了，我真不是死肥宅）
var cuteEmojis = []string{
	"(◕ᴗ◕✿)", "₍˄·͈༝·͈˄₎◞ ̑̑", "ᗢ₊˚⊹", "꒰ᐢ⸝⸝•༝•⸝⸝ᐢ꒱",
	"(●'◡'●)", "(｡♥‿♥｡)", "( ˊᵕˋ )♡.°⑅", "ପ(๑•ᴗ•๑)ଓ ♡",
}

func init() {
	scanner := bufio.NewScanner(strings.NewReader(luckMessagesData))
	var currentRange string
	var messages []string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if currentRange != "" {
				luckMessages[currentRange] = messages
			}
			currentRange = line[1 : len(line)-1]
			messages = make([]string, 0)
			continue
		}

		if currentRange != "" {
			messages = append(messages, line)
		}
	}

	if currentRange != "" {
		luckMessages[currentRange] = messages
	}
}

func RegisterLuckHandler() {
	zero.OnCommand("jrrp").
		Handle(func(ctx *zero.Ctx) {
			userID := strconv.FormatInt(ctx.Event.UserID, 10)
			today := time.Now().Format("2006-01-02")

			val, exists, err := storage.GetUserLuck(userID, today)
			if err != nil {
				ctx.Send(fmt.Sprintf("获取今日人品值时出错啦，请将错误信息反馈给管理员哦\n\n%v", err))
				return
			}

			msg := generateLuckMessage(val)

			if exists {
				r := rand.New(rand.NewSource(time.Now().UnixNano()))
				emoji := cuteEmojis[r.Intn(len(cuteEmojis))]

				msg = fmt.Sprintf("你今天已经找井盖酱获取过今日人品了哦 %s\n\n%s", emoji, msg)
			}

			if !exists {
				val = generateLuckValue(ctx.Event.UserID, today)

				if err := storage.RecordUserLuck(userID, today, val); err != nil {
					ctx.Send(fmt.Sprintf("记录人品值时出错啦，请将错误信息反馈给管理员哦\n\n%v", err))
					return
				}

				msg = generateLuckMessage(val)
			}

			ctx.Send(msg)
		})
}

func generateLuckValue(uid int64, day string) int {
	h := fnv.New64a()
	h.Write([]byte(fmt.Sprintf("%d|%s", uid, day)))
	seed := h.Sum64()

	r := rand.New(rand.NewSource(int64(seed)))
	return r.Intn(101)
}

func getLuckRange(value int) string {
	switch {
	case value <= 20:
		return "0-20"
	case value <= 40:
		return "21-40"
	case value <= 60:
		return "41-60"
	case value <= 80:
		return "61-80"
	default:
		return "81-100"
	}
}

func generateLuckMessage(value int) string {
	rangeKey := getLuckRange(value)
	messages := luckMessages[rangeKey]
	if len(messages) == 0 {
		return fmt.Sprintf("%d", value)
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	msg := messages[r.Intn(len(messages))]

	return strings.ReplaceAll(msg, "{value}", strconv.Itoa(value))
}
