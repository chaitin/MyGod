package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"app/internal/bootstrap"
	"app/internal/config"
	"app/internal/httpserver"
	"app/internal/store"
)

// @title AI 原生企业协作服务 API
// @version 0.1.0
// @description 私有部署企业协作产品的服务端 API。当前阶段包含管理员登录、管理员创建普通用户、普通用户登录。
// @BasePath /
//
// @securityDefinitions.apikey UserSession
// @in header
// @name Cookie
// @description 使用 user_session=<token> 格式的会话 Cookie。
func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config", "error", err)
		os.Exit(1)
	}

	db, err := store.OpenPostgres(cfg.Database.DSN)
	if err != nil {
		logger.Error("connect database", "error", err)
		os.Exit(1)
	}
	if err := store.Ping(db); err != nil {
		logger.Error("ping database", "error", err)
		os.Exit(1)
	}

	if err := bootstrap.Run(context.Background(), db, cfg); err != nil {
		logger.Error("bootstrap server", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	router := httpserver.NewRouterWithTaskReminderWorker(ctx, db, cfg)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := router.Shutdown(shutdownCtx); err != nil {
			logger.Error("shutdown server", "error", err)
		}
	}()
	logger.Info("server starting", "addr", cfg.Server.Addr)
	if err := router.Start(cfg.Server.Addr); err != nil && err != http.ErrServerClosed {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
