package dashboard

import (
	"context"
	"time"

	"app/internal/store"

	"gorm.io/gorm"
)

type PresencePort interface {
	OnlineStatus([]string) map[string]bool
}

type Dependencies struct {
	DB       *gorm.DB
	Presence PresencePort
	Now      func() time.Time
}

type Service struct {
	db       *gorm.DB
	presence PresencePort
	now      func() time.Time
}

type Stats struct {
	TotalUsers             int64
	VisitedUsers24Hours    int64
	VisitedUsers7Days      int64
	OnlineUsers            int64
	Messages24Hours        int64
	Messages7Days          int64
	ActiveConversations24H int64
	ActiveConversations7D  int64
}

type AdminService interface {
	GetStats(context.Context) (Stats, error)
}

func NewService(deps Dependencies) *Service {
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{db: deps.DB, presence: deps.Presence, now: now}
}

func (s *Service) GetStats(ctx context.Context) (Stats, error) {
	now := s.now().UTC()
	cutoff24Hours := now.Add(-24 * time.Hour)
	cutoff7Days := now.AddDate(0, 0, -7)

	var users []struct {
		ID           string
		LastOnlineAt *time.Time
	}
	if err := s.db.WithContext(ctx).Model(&store.User{}).
		Select("id", "last_online_at").Find(&users).Error; err != nil {
		return Stats{}, err
	}

	userIDs := make([]string, 0, len(users))
	for _, user := range users {
		userIDs = append(userIDs, user.ID)
	}
	onlineStatus := map[string]bool{}
	if s.presence != nil {
		onlineStatus = s.presence.OnlineStatus(userIDs)
	}

	stats := Stats{TotalUsers: int64(len(users))}
	for _, user := range users {
		online := onlineStatus[user.ID]
		if online {
			stats.OnlineUsers++
		}
		if online || visitedSince(user.LastOnlineAt, cutoff24Hours) {
			stats.VisitedUsers24Hours++
		}
		if online || visitedSince(user.LastOnlineAt, cutoff7Days) {
			stats.VisitedUsers7Days++
		}
	}

	var err error
	if stats.Messages24Hours, err = s.countMessagesSince(ctx, cutoff24Hours); err != nil {
		return Stats{}, err
	}
	if stats.Messages7Days, err = s.countMessagesSince(ctx, cutoff7Days); err != nil {
		return Stats{}, err
	}
	if stats.ActiveConversations24H, err = s.countActiveConversationsSince(ctx, cutoff24Hours); err != nil {
		return Stats{}, err
	}
	if stats.ActiveConversations7D, err = s.countActiveConversationsSince(ctx, cutoff7Days); err != nil {
		return Stats{}, err
	}

	return stats, nil
}

func (s *Service) countMessagesSince(ctx context.Context, cutoff time.Time) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&store.Message{}).
		Where("created_at >= ?", cutoff).Count(&count).Error
	return count, err
}

func (s *Service) countActiveConversationsSince(ctx context.Context, cutoff time.Time) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&store.Conversation{}).
		Where("last_message_at >= ?", cutoff).Count(&count).Error
	return count, err
}

func visitedSince(lastOnlineAt *time.Time, cutoff time.Time) bool {
	return lastOnlineAt != nil && !lastOnlineAt.Before(cutoff)
}
