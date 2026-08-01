package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/sirupsen/logrus"

	"bankapi/internal/config"
	"bankapi/internal/handler"
	"bankapi/internal/repository"
	"bankapi/internal/service"
)

func main() {
	cfg := config.Load()
	log := newLogger(cfg.LogLevel)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// --- Слой данных ---
	db, err := repository.NewPostgres(cfg, log)
	if err != nil {
		log.Fatalf("не удалось подключиться к БД: %v", err)
	}
	defer db.Close()

	if err := repository.RunMigrations(ctx, db, cfg.MigrationsDir, log); err != nil {
		log.Fatalf("не удалось применить миграции: %v", err)
	}
	repos := repository.New(db)

	// --- Слой бизнес-логики ---
	services := service.New(repos, cfg, log)

	// Шедулер обработки платежей по кредитам (каждые N часов).
	scheduler := service.NewScheduler(services.Credit, cfg.SchedulerInterval, log)
	scheduler.Start(ctx)

	// --- HTTP-слой ---
	srv := &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           handler.NewRouter(services, cfg, log),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Infof("HTTP-сервер слушает порт %s", cfg.AppPort)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("ошибка HTTP-сервера: %v", err)
		}
	}()

	// --- Graceful shutdown ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("получен сигнал завершения, останавливаем сервис")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Errorf("ошибка остановки HTTP-сервера: %v", err)
	}
	cancel()
	scheduler.Wait()
	log.Info("сервис остановлен")
}

func newLogger(level string) *logrus.Logger {
	log := logrus.New()
	log.SetOutput(os.Stdout)
	log.SetFormatter(&logrus.JSONFormatter{TimestampFormat: time.RFC3339})
	lvl, err := logrus.ParseLevel(level)
	if err != nil {
		lvl = logrus.InfoLevel
	}
	log.SetLevel(lvl)
	return log
}
