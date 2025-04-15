//go:build !test

package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"PakuchiBot/internal/bot"
	"PakuchiBot/internal/handler"
	"PakuchiBot/internal/repository"
	"PakuchiBot/internal/scheduler"
	"PakuchiBot/internal/storage"
	"PakuchiBot/internal/utils"

	zero "github.com/wdvxdr1123/ZeroBot"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("get panic: %v\n", r)
			os.Exit(1)
		}
	}()

	if err := bot.InitConfig(); err != nil {
		log.Fatalf("failed to init config: %v", err)
	}

	if err := bot.InitBot(); err != nil {
		log.Fatalf("failed to init bot: %v", err)
	}

	time.Sleep(2 * time.Second)

	crypto, err := utils.NewTokenCrypto(bot.Config.Storage.EncryptionKey)
	if err != nil {
		log.Fatalf("failed to create crypto tool: %v", err)
	}

	scheduler := scheduler.NewScheduler(
		repository.NewUserRepository(bot.DB),
		repository.NewSignRepository(bot.DB),
		repository.NewNotifyRepository(bot.DB),
		crypto,
		zero.GetBot(bot.Config.Bot.SelfID),
		time.Duration(bot.Config.Scheduler.CheckInterval)*time.Second,
		bot.Config.Scheduler.MaxRetries,
	)

	scheduler.Start()
	defer scheduler.Stop()

	// Default Plugin
	handler.RegisterPingHandler()
	handler.RegisterLuckHandler()

	// MGClub
	mgHandler := handler.NewMGClubHandler(bot.UserRepo, bot.NotifyRepo, bot.TokenCrypto)
	mgHandler.Register()

	handler.RegisterHumanLikeHandler()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	if err := storage.CloseDB(); err != nil {
		log.Printf("failed to close db: %v\n", err)
	}

	log.Println("program exited")
}
