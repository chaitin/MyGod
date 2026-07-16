package message

import (
	"context"
	"fmt"
	"time"

	"app/internal/store"

	"gorm.io/gorm"
)

type storedMessagePageQuery struct {
	ConversationID   string
	VisibleFromSeq   int64
	BeforeSeq        *int64
	AfterSeq         *int64
	BeforeOrEqualSeq *int64
	Limit            int
	Descending       bool
}

func (s *Service) loadStoredMessagePage(
	ctx context.Context,
	db *gorm.DB,
	query storedMessagePageQuery,
) ([]store.Message, error) {
	if !store.MessagePartitioningEnabled(db) {
		statement := applyOnlineStoredMessageWindow(db.WithContext(ctx)).Where(
			"conversation_id = ? AND deleted_at IS NULL AND seq >= ?",
			query.ConversationID, query.VisibleFromSeq,
		)
		statement = applyStoredMessagePageBounds(statement, query)
		statement = statement.Limit(query.Limit)
		if query.Descending {
			statement = statement.Order("seq DESC")
		} else {
			statement = statement.Order("seq ASC")
		}
		var messages []store.Message
		if err := statement.Find(&messages).Error; err != nil {
			return nil, err
		}
		return messages, nil
	}

	statement := applyOnlineStoredMessageWindow(db.WithContext(ctx).Model(&store.MessageRegistry{})).Where(
		"conversation_id = ? AND deleted_at IS NULL AND seq >= ?",
		query.ConversationID, query.VisibleFromSeq,
	)
	statement = applyStoredMessagePageBounds(statement, query)
	statement = statement.Limit(query.Limit)
	if query.Descending {
		statement = statement.Order("seq DESC")
	} else {
		statement = statement.Order("seq ASC")
	}
	var registries []store.MessageRegistry
	if err := statement.Find(&registries).Error; err != nil {
		return nil, err
	}
	messagesByID, err := store.LoadMessagesByRegistry(ctx, db, registries)
	if err != nil {
		return nil, err
	}
	messages := make([]store.Message, 0, len(registries))
	for _, registry := range registries {
		message, ok := messagesByID[registry.ID]
		if !ok {
			return nil, fmt.Errorf("message %s is registered but missing from partition %d", registry.ID, registry.PartitionYear)
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func applyOnlineStoredMessageWindow(db *gorm.DB) *gorm.DB {
	now := time.Now().UTC()
	if store.MessagePartitioningEnabled(db) {
		return db.Where(
			"partition_year >= ? AND partition_year <= ?",
			store.MessageMinimumOnlineYear(now), store.MessageMaximumOnlineYear(now),
		)
	}
	return db.Where("created_at >= ? AND created_at < ?", store.MessageOnlineCutoff(now), store.MessageOnlineEnd(now))
}

func applyStoredMessagePageBounds(db *gorm.DB, query storedMessagePageQuery) *gorm.DB {
	if query.BeforeSeq != nil {
		db = db.Where("seq < ?", *query.BeforeSeq)
	}
	if query.AfterSeq != nil {
		db = db.Where("seq > ?", *query.AfterSeq)
	}
	if query.BeforeOrEqualSeq != nil {
		db = db.Where("seq <= ?", *query.BeforeOrEqualSeq)
	}
	return db
}

func messageStorageContext(db *gorm.DB) context.Context {
	if db != nil && db.Statement != nil && db.Statement.Context != nil {
		return db.Statement.Context
	}
	return context.Background()
}
