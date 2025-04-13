package handler

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"PakuchiBot/internal/mgclub"
	"PakuchiBot/internal/repository"
	"PakuchiBot/internal/utils"

	"github.com/sirupsen/logrus"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

type MGClubHandler struct {
	userRepo   *repository.UserRepository
	notifyRepo *repository.NotifyRepository
	crypto     *utils.TokenCrypto
}

func NewMGClubHandler(
	userRepo *repository.UserRepository,
	notifyRepo *repository.NotifyRepository,
	crypto *utils.TokenCrypto,
) *MGClubHandler {
	return &MGClubHandler{
		userRepo:   userRepo,
		notifyRepo: notifyRepo,
		crypto:     crypto,
	}
}

func (h *MGClubHandler) Register() {
	zero.OnCommand("255token").
		Handle(func(ctx *zero.Ctx) {
			args := ctx.State["args"].(string)
			token := strings.TrimSpace(args)
			if token == "" {
				ctx.Send("你还没有绑定过毛吧账号哦，请先提供要绑定的毛吧 token 喵\n\n格式： " + zero.BotConfig.CommandPrefix + "255token <token值>")
				return
			}

			reqCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			encryptedToken, err := h.crypto.Encrypt(token)
			if err != nil {
				ctx.Send(fmt.Sprintf("加密 token 时出错啦，请将错误信息反馈给管理员哦\n\n%v", err))
				return
			}

			userID := fmt.Sprintf("%d", ctx.Event.UserID)

			_, err = h.userRepo.GetByUserID(reqCtx, userID)
			if err == nil {
				if err := h.userRepo.Update(reqCtx, userID, encryptedToken); err != nil {
					ctx.Send(fmt.Sprintf("更新 token 时出错啦，请将错误信息反馈给管理员哦\n\n%v", err))
					return
				}
				ctx.Send("token 更新成功喵")
			} else {
				if err := h.userRepo.Create(reqCtx, userID, encryptedToken); err != nil {
					ctx.Send(fmt.Sprintf("创建用户时出错啦，请将错误信息反馈给管理员哦\n\n%v", err))
					return
				}
				ctx.Send("token 绑定成功喵")
			}

			var groupID *int64
			if ctx.Event.GroupID != 0 {
				groupID = &ctx.Event.GroupID
			}
			if err := h.notifyRepo.UpsertSetting(reqCtx, userID, groupID); err != nil {
				log.Printf("failed to save notification settings: %v", err)
			}
		})

	zero.OnCommand("255sign").
		Handle(func(ctx *zero.Ctx) {
			reqCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			userID := fmt.Sprintf("%d", ctx.Event.UserID)

			user, err := h.userRepo.GetByUserID(reqCtx, userID)
			if err != nil {
				log.Printf("failed to get user by userid:%v, err:%v", userID, err)
				ctx.Send("你还没有绑定过毛吧账号哦，请先使用 " + zero.BotConfig.CommandPrefix + "255token <token值> 绑定你的毛吧账号")
				return
			}

			token, err := h.crypto.Decrypt(user.Token)
			if err != nil {
				ctx.Send(fmt.Sprintf("解密 token 时出错啦，请将错误信息反馈给管理员哦\n\n%v", err))
				return
			}

			result, err := mgclub.ProcessSign(token)
			if err != nil {
				ctx.Send(fmt.Sprintf("签到时出错啦，请将错误信息反馈给管理员哦\n\n%v", err))
				return
			}

			if result.ImageData != nil {
				ctx.Send(message.Message{
					message.Text(result.Message),
					message.ImageBytes(result.ImageData),
				})
			} else {
				ctx.Send(result.Message)
			}
		})

	zero.OnCommand("255info").Handle(func(ctx *zero.Ctx) {
		reqCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		userID := fmt.Sprintf("%d", ctx.Event.UserID)

		user, err := h.userRepo.GetByUserID(reqCtx, userID)
		if err != nil {
			log.Printf("failed to get user by userid:%v, err:%v", userID, err)
			ctx.Send("你还没有绑定 token 喵，请先使用 " + zero.BotConfig.CommandPrefix + "255token <token值> 绑定你的毛吧账号")
			return
		}

		token, err := h.crypto.Decrypt(user.Token)
		if err != nil {
			ctx.Send(fmt.Sprintf("token 解密时出错啦，请将错误信息反馈给管理员哦\n\n%v", err))
			return
		}

		client := mgclub.NewClient()

		info, err := client.GetUserInfo(token)
		if err != nil {
			ctx.Send(fmt.Sprintf("获取用户信息时出错啦，请将错误信息反馈给管理员哦\n\n%v", err))
			return
		}

		cardImage, err := utils.GenerateUserCard(
			info.Nickname,
			info.Sign,
			info.Avatar,
			info.UID,
			info.Exp,
			info.Contribution,
			info.Location,
			info.ParseBirthday(),
		)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"user_id":  userID,
				"nickname": info.Nickname,
				"error":    err,
			}).Error("failed to generate user info card")

			reply := fmt.Sprintf(
				"用户信息:\n"+
					"UID: %d\n"+
					"昵称: %s\n"+
					"签名: %s\n"+
					"经验值: %d\n"+
					"贡献值: %d\n"+
					"地区: %s\n"+
					"生日: %s\n",
				info.UID,
				info.Nickname,
				info.Sign,
				info.Exp,
				info.Contribution,
				info.Location,
				info.ParseBirthday(),
			)
			ctx.Send(reply)
			return
		}

		msgID := ctx.Send(message.Image(cardImage))
		if msgID.ID() == 0 {
			logrus.WithFields(logrus.Fields{
				"user_id":  userID,
				"nickname": info.Nickname,
			}).Error("failed to send user info card")

			reply := fmt.Sprintf(
				"用户信息:\n"+
					"UID: %d\n"+
					"昵称: %s\n"+
					"签名: %s\n"+
					"经验值: %d\n"+
					"贡献值: %d\n"+
					"地区: %s\n"+
					"生日: %s\n",
				info.UID,
				info.Nickname,
				info.Sign,
				info.Exp,
				info.Contribution,
				info.Location,
				info.ParseBirthday(),
			)
			ctx.Send(reply)
		}
	})
}
