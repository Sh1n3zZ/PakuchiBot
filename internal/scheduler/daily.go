package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync/atomic"

	"PakuchiBot/internal/mgclub"
	"PakuchiBot/internal/repository"
	"PakuchiBot/internal/utils"

	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

var (
	ErrSignInProgress = errors.New("sign in is already in progress")
)

type SignTask struct {
	userRepo   *repository.UserRepository
	signRepo   *repository.SignRepository
	notifyRepo *repository.NotifyRepository
	crypto     *utils.TokenCrypto
	bot        *zero.Ctx
	isRunning  atomic.Bool
	maxRetries int
}

func NewSignTask(
	userRepo *repository.UserRepository,
	signRepo *repository.SignRepository,
	notifyRepo *repository.NotifyRepository,
	crypto *utils.TokenCrypto,
	bot *zero.Ctx,
	maxRetries int,
) *SignTask {
	return &SignTask{
		userRepo:   userRepo,
		signRepo:   signRepo,
		notifyRepo: notifyRepo,
		crypto:     crypto,
		bot:        bot,
		maxRetries: maxRetries,
	}
}

func (t *SignTask) Run(ctx context.Context) {
	if !t.isRunning.CompareAndSwap(false, true) {
		log.Printf("check-in tasks are in progress")
		return
	}
	defer t.isRunning.Store(false)

	users, err := t.userRepo.GetAllUsers(ctx)
	if err != nil {
		log.Printf("failed to get user list: %v", err)
		return
	}

	userIDs := make([]string, len(users))
	for i, user := range users {
		userIDs[i] = user.UserID
	}
	if err := t.signRepo.InitDailyRecords(ctx, userIDs); err != nil {
		log.Printf("failed to initialize sign-in log: %v", err)
		return
	}

	records, err := t.signRepo.GetPendingRecords(ctx, t.maxRetries)
	if err != nil {
		log.Printf("failed to get pending check-in records: %v", err)
		return
	}

	if len(records) == 0 {
		return
	}

	userMap := make(map[string]repository.User)
	for _, user := range users {
		userMap[user.UserID] = user
	}

	for _, record := range records {
		select {
		case <-ctx.Done():
			log.Printf("check-in tasks are canceled")
			return
		default:
			user, ok := userMap[record.UserID]
			if !ok {
				log.Printf("user %s not found", record.UserID)
				continue
			}
			t.processSignRecord(ctx, user, record)
		}
	}
}

func (t *SignTask) processSignRecord(ctx context.Context, user repository.User, record repository.SignRecord) {
	token, err := t.crypto.Decrypt(user.Token)
	if err != nil {
		log.Printf("user %s token decryption failed: %v", user.UserID, err)
		t.notifyUser(ctx, user.UserID, fmt.Sprintf("自动签到时 token 解密失败啦，请将错误信息反馈给管理员哦\n\n%v", err), nil)
		t.signRepo.UpdateStatus(ctx, record.ID, repository.SignStatusFailed)
		return
	}

	result, err := mgclub.ProcessSign(token)
	if err != nil {
		log.Printf("user %s sign in failed: %v", user.UserID, err)
		t.notifyUser(ctx, user.UserID, fmt.Sprintf("自动签到失败啦，请将错误信息反馈给管理员哦\n\n%v", err), nil)
		t.signRepo.UpdateStatus(ctx, record.ID, repository.SignStatusFailed)
		return
	}

	t.signRepo.UpdateStatus(ctx, record.ID, repository.SignStatusSuccess)
	t.notifyUser(ctx, user.UserID, result.Message, result.ImageData)
}

func (t *SignTask) notifyUser(ctx context.Context, userID string, msg string, imageData []byte) {
	if t.bot == nil {
		log.Printf("bot instance not initialized, cannot send notification")
		return
	}

	setting, err := t.notifyRepo.GetSetting(ctx, userID)
	if err != nil {
		log.Printf("failed to get user %s notification settings: %v", userID, err)
		return
	}

	qqID, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		log.Printf("failed to parse user ID: %v", err)
		return
	}

	if setting.GroupID != nil {
		msgElements := []message.MessageSegment{
			message.At(qqID),
			message.Text("\n" + msg),
		}

		if imageData != nil {
			msgElements = append(msgElements, message.ImageBytes(imageData))
		}

		t.bot.SendGroupMessage(*setting.GroupID, message.Message(msgElements))
	} else {
		if imageData != nil {
			t.bot.SendPrivateMessage(qqID, message.Message{
				message.Text(msg),
				message.ImageBytes(imageData),
			})
		} else {
			t.bot.SendPrivateMessage(qqID, message.Text(msg))
		}
	}
}
