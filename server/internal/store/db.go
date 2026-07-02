package store

import (
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func OpenPostgres(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	return db, nil
}

func AutoMigrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&User{},
		&AdminSession{},
		&UserSession{},
		&Conversation{},
		&ConversationMember{},
		&AppSettings{},
	); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}

	return nil
}
