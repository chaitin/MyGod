package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestRunGooseMigrationsAppliesSQLMigrations(t *testing.T) {
	db := openUnmigratedTestDB(t)
	migrationsDir := t.TempDir()
	writeTestMigration(t, migrationsDir, "00001_create_widgets.sql", `-- +goose Up
CREATE TABLE widgets (
  id integer PRIMARY KEY,
  name text NOT NULL
);
INSERT INTO widgets (id, name) VALUES (1, 'alpha');

-- +goose Down
DROP TABLE widgets;
`)

	if err := RunGooseMigrations(db, "sqlite3", migrationsDir); err != nil {
		t.Fatalf("RunGooseMigrations() error = %v", err)
	}

	var widgetCount int64
	if err := db.Table("widgets").Count(&widgetCount).Error; err != nil {
		t.Fatalf("count widgets: %v", err)
	}
	if widgetCount != 1 {
		t.Fatalf("widgets count = %d, want 1", widgetCount)
	}

	var versionCount int64
	if err := db.Table("goose_db_version").Where("version_id = ?", 1).Count(&versionCount).Error; err != nil {
		t.Fatalf("count goose version: %v", err)
	}
	if versionCount != 1 {
		t.Fatalf("goose version count = %d, want 1", versionCount)
	}
}

func TestRunGooseMigrationsReturnsMigrationErrors(t *testing.T) {
	db := openUnmigratedTestDB(t)
	missingDir := filepath.Join(t.TempDir(), "missing")

	err := RunGooseMigrations(db, "sqlite3", missingDir)
	if err == nil {
		t.Fatal("RunGooseMigrations() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "run goose migrations") {
		t.Fatalf("RunGooseMigrations() error = %q, want run goose migrations context", err.Error())
	}
}

func TestFindMigrationsDirFindsRepositoryMigrations(t *testing.T) {
	dir, err := FindMigrationsDir()
	if err != nil {
		t.Fatalf("FindMigrationsDir() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "00001_init_schema.sql")); err != nil {
		t.Fatalf("stat first migration: %v", err)
	}
}

func openUnmigratedTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}

	return db
}

func writeTestMigration(t *testing.T, dir string, name string, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write test migration: %v", err)
	}
}
