package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pressly/goose/v3"
	"gorm.io/gorm"
)

var gooseMigrationMu sync.Mutex

func RunPostgresMigrations(db *gorm.DB, migrationsDir string) error {
	return RunGooseMigrations(db, "postgres", migrationsDir)
}

func RunGooseMigrations(db *gorm.DB, dialect string, migrationsDir string) error {
	dialect = strings.TrimSpace(dialect)
	if dialect == "" {
		return fmt.Errorf("goose dialect is required")
	}
	migrationsDir = strings.TrimSpace(migrationsDir)
	if migrationsDir == "" {
		return fmt.Errorf("goose migrations dir is required")
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get sql database: %w", err)
	}

	gooseMigrationMu.Lock()
	defer gooseMigrationMu.Unlock()

	if err := goose.SetDialect(dialect); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	if err := goose.Up(sqlDB, migrationsDir); err != nil {
		return fmt.Errorf("run goose migrations: %w", err)
	}

	return nil
}

func FindMigrationsDir() (string, error) {
	dir, ok := findDirContaining("migrations", "00001_init_schema.sql")
	if !ok {
		return "", fmt.Errorf("find migrations dir: migrations/00001_init_schema.sql not found from current directory")
	}

	return dir, nil
}

func findDirContaining(relativeDir string, requiredFile string) (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}

	for {
		candidate := filepath.Join(dir, relativeDir)
		statPath := candidate
		if requiredFile != "" {
			statPath = filepath.Join(candidate, requiredFile)
		}
		if _, err := os.Stat(statPath); err == nil {
			return candidate, true
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
