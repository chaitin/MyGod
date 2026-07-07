package bootstrap

import (
	"context"
	"fmt"
	"time"

	"app/internal/appregistry"
	"app/internal/config"
	"app/internal/objectstore"
	"app/internal/store"

	"gorm.io/gorm"
)

const storageBootstrapTimeout = 2 * time.Minute

func Run(ctx context.Context, db *gorm.DB, cfg config.Config) error {
	if err := runMigrations(db); err != nil {
		return err
	}
	if err := bootstrapApps(db, cfg); err != nil {
		return err
	}
	if err := bootstrapStorage(ctx, cfg.Storage); err != nil {
		return err
	}

	return nil
}

func runMigrations(db *gorm.DB) error {
	migrationsDir, err := store.FindMigrationsDir()
	if err != nil {
		return fmt.Errorf("find migrations: %w", err)
	}
	if err := store.RunPostgresMigrations(db, migrationsDir); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	return nil
}

func bootstrapApps(db *gorm.DB, cfg config.Config) error {
	if _, err := appregistry.EnsureGoddessApp(db, cfg.Apps); err != nil {
		return fmt.Errorf("bootstrap goddess app: %w", err)
	}

	return nil
}

func bootstrapStorage(ctx context.Context, cfg config.StorageConfig) error {
	if cfg.Provider == "" {
		return nil
	}

	storageCtx, cancel := context.WithTimeout(ctx, storageBootstrapTimeout)
	defer cancel()

	storageClient, err := objectstore.New(storageCtx, cfg)
	if err != nil {
		return fmt.Errorf("configure storage: %w", err)
	}
	if err := storageClient.Bootstrap(storageCtx); err != nil {
		return fmt.Errorf("bootstrap storage: %w", err)
	}

	return nil
}
