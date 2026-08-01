package service

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
)

// Scheduler периодически запускает обработку платежей по кредитам.
type Scheduler struct {
	credits  *CreditService
	interval time.Duration
	log      *logrus.Logger
	done     chan struct{}
}

// NewScheduler создаёт шедулер с заданным интервалом (по умолчанию 12 часов).
func NewScheduler(credits *CreditService, interval time.Duration, log *logrus.Logger) *Scheduler {
	if interval <= 0 {
		interval = 12 * time.Hour
	}
	return &Scheduler{
		credits:  credits,
		interval: interval,
		log:      log,
		done:     make(chan struct{}),
	}
}

// Start запускает фоновый цикл до отмены контекста.
func (s *Scheduler) Start(ctx context.Context) {
	s.log.Infof("шедулер запущен, интервал %s", s.interval)
	go func() {
		defer close(s.done)
		// Первый прогон сразу после старта, чтобы подобрать просрочки простоя.
		s.run(ctx)

		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				s.log.Info("шедулер остановлен")
				return
			case <-ticker.C:
				s.run(ctx)
			}
		}
	}()
}

// Wait блокируется до полной остановки шедулера.
func (s *Scheduler) Wait() { <-s.done }

func (s *Scheduler) run(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			s.log.Errorf("паника в шедулере: %v", r)
		}
	}()
	runCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	s.credits.ProcessDuePayments(runCtx)
}
