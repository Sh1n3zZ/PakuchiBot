package scheduler

import (
	"context"
	"errors"
	"log"
	"time"

	"PakuchiBot/internal/repository"
	"PakuchiBot/internal/utils"

	zero "github.com/wdvxdr1123/ZeroBot"
)

var (
	ErrSchedulerRunning = errors.New("scheduler is already running")
	ErrSchedulerStopped = errors.New("scheduler is not running")
)

type Scheduler struct {
	signTask    *SignTask
	checkTicker *time.Ticker
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewScheduler(
	userRepo *repository.UserRepository,
	signRepo *repository.SignRepository,
	notifyRepo *repository.NotifyRepository,
	crypto *utils.TokenCrypto,
	bot *zero.Ctx,
	checkInterval time.Duration,
	maxRetries int,
) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		signTask: NewSignTask(
			userRepo,
			signRepo,
			notifyRepo,
			crypto,
			bot,
			maxRetries,
		),
		checkTicker: time.NewTicker(checkInterval),
		ctx:         ctx,
		cancel:      cancel,
	}
}

func (s *Scheduler) Start() {
	log.Printf("scheduler started")
	go func() {
		for {
			select {
			case <-s.ctx.Done():
				log.Printf("scheduler stopped")
				return
			case <-s.checkTicker.C:
				s.signTask.Run(s.ctx)
			}
		}
	}()
}

func (s *Scheduler) Stop() {
	s.cancel()
	s.checkTicker.Stop()
}
