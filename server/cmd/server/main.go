package main

import (
	"log/slog"
	"net/http"
	"os"

	"app/internal/config"
	"app/internal/httpserver"
	"app/internal/store"
)

// @title AI 原生企业协作服务 API
// @version 0.1.0
// @description 私有部署企业协作产品的服务端 API。当前阶段包含管理员登录、管理员创建普通用户、普通用户登录。
// @BasePath /
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

	migrationsDir, err := store.FindMigrationsDir()
	if err != nil {
		logger.Error("find migrations", "error", err)
		os.Exit(1)
	}
	if err := store.RunPostgresMigrations(db, migrationsDir); err != nil {
		logger.Error("migrate database", "error", err)
		os.Exit(1)
	}

	router := httpserver.NewRouter(db, cfg)
	logger.Info("server starting", "addr", cfg.Server.Addr)
	if err := router.Start(cfg.Server.Addr); err != nil && err != http.ErrServerClosed {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
