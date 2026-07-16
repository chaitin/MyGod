package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

const (
	MessageHashPartitionCount   = 32
	MessageOnlineRetentionYears = 2
	messageMinimumYear          = 1970
	messageMaximumYear          = 9999
)

func MessagePartitionYear(value time.Time) int {
	return value.UTC().Year()
}

func MessageMinimumOnlineYear(now time.Time) int {
	return MessagePartitionYear(now) - MessageOnlineRetentionYears + 1
}

func MessageMaximumOnlineYear(now time.Time) int {
	return MessagePartitionYear(now)
}

func MessageOnlineCutoff(now time.Time) time.Time {
	return time.Date(MessageMinimumOnlineYear(now), time.January, 1, 0, 0, 0, 0, time.UTC)
}

func MessageOnlineEnd(now time.Time) time.Time {
	return time.Date(MessageMaximumOnlineYear(now)+1, time.January, 1, 0, 0, 0, 0, time.UTC)
}

func MessagePartitionYearBounds(year int) (time.Time, time.Time, error) {
	if err := validateMessagePartitionYear(year); err != nil {
		return time.Time{}, time.Time{}, err
	}
	start := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(1, 0, 0), nil
}

func EnsureMessagePartitionWindow(ctx context.Context, db *gorm.DB, now time.Time) error {
	if db == nil {
		return errors.New("database is required")
	}
	if db.Dialector.Name() != "postgres" {
		return nil
	}
	currentYear := MessagePartitionYear(now)
	for year := MessageMinimumOnlineYear(now); year <= currentYear+2; year++ {
		if err := EnsureMessageYearPartitions(ctx, db, year); err != nil {
			return err
		}
	}
	return nil
}

func EnsureMessageYearPartitions(ctx context.Context, db *gorm.DB, year int) error {
	if db == nil {
		return errors.New("database is required")
	}
	if err := validateMessagePartitionYear(year); err != nil {
		return err
	}
	if db.Dialector.Name() != "postgres" {
		return nil
	}
	if err := db.WithContext(ctx).Exec("SELECT ensure_message_year_partitions(?)", year).Error; err != nil {
		return fmt.Errorf("ensure message partitions for %d: %w", year, err)
	}
	return nil
}

func MessagePartitioningEnabled(db *gorm.DB) bool {
	return db != nil && db.Dialector.Name() == "postgres"
}

func ScopeMessagePartition(ctx context.Context, db *gorm.DB, year int) (*gorm.DB, error) {
	if db == nil {
		return nil, errors.New("database is required")
	}
	start, end, err := MessagePartitionYearBounds(year)
	if err != nil {
		return nil, err
	}
	return db.WithContext(ctx).Model(&Message{}).
		Where("created_at >= ? AND created_at < ?", start, end), nil
}

func LoadMessageByRegistry(ctx context.Context, db *gorm.DB, registry MessageRegistry) (Message, error) {
	scope, err := ScopeMessagePartition(ctx, db, int(registry.PartitionYear))
	if err != nil {
		return Message{}, err
	}
	var message Message
	if err := scope.Where("conversation_id = ? AND id = ?", registry.ConversationID, registry.ID).
		Take(&message).Error; err != nil {
		return Message{}, err
	}
	return message, nil
}

func LoadMessagesByRegistry(ctx context.Context, db *gorm.DB, registries []MessageRegistry) (map[string]Message, error) {
	result := make(map[string]Message, len(registries))
	if len(registries) == 0 {
		return result, nil
	}
	if !MessagePartitioningEnabled(db) {
		ids := make([]string, 0, len(registries))
		for _, registry := range registries {
			ids = append(ids, registry.ID)
		}
		var messages []Message
		if err := db.WithContext(ctx).Where("id IN ?", ids).Find(&messages).Error; err != nil {
			return nil, err
		}
		for _, message := range messages {
			result[message.ID] = message
		}
		return result, nil
	}

	type groupKey struct {
		ConversationID string
		Year           int
	}
	groups := make(map[groupKey][]string)
	for _, registry := range registries {
		key := groupKey{ConversationID: registry.ConversationID, Year: int(registry.PartitionYear)}
		groups[key] = append(groups[key], registry.ID)
	}
	for key, ids := range groups {
		scope, err := ScopeMessagePartition(ctx, db, key.Year)
		if err != nil {
			return nil, err
		}
		var messages []Message
		if err := scope.Where("conversation_id = ? AND id IN ?", key.ConversationID, ids).
			Find(&messages).Error; err != nil {
			return nil, err
		}
		for _, message := range messages {
			result[message.ID] = message
		}
	}
	return result, nil
}

func validateMessagePartitionYear(year int) error {
	if year < messageMinimumYear || year > messageMaximumYear {
		return fmt.Errorf("message partition year %d is outside the supported range", year)
	}
	return nil
}
