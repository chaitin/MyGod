package httpserver

import (
	"time"

	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func createPersonalProject(db *gorm.DB, user store.User, now time.Time) error {
	project := store.Project{
		ID:              uuid.NewString(),
		Name:            "个人工作区",
		Description:     "",
		Avatar:          "",
		OwnerUserID:     user.ID,
		CreatedByUserID: user.ID,
		IsPersonal:      true,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	return db.Create(&project).Error
}
