package conversation

import (
	"context"
	"errors"
	"sort"
	"strings"

	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (s *Service) CreateDirect(ctx context.Context, cmd CreateDirectCommand) (OpenResult, error) {
	current := actorUser(cmd.Actor)
	targetID, err := normalizeDirectUserID(cmd.UserID, current.ID)
	if err != nil {
		return OpenResult{}, invalidRequest(err.Error(), err)
	}
	db := s.db
	var target store.User
	err = db.First(&target, "id = ? AND status = ?", targetID, store.UserStatusActive).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return OpenResult{}, invalidRequest("用户不存在或已禁用", err)
	}
	if err != nil {
		return OpenResult{}, internalError(err)
	}
	conversation, created, err := s.getOrCreateDirect(db, current, target)
	if err != nil {
		return OpenResult{}, internalError(err)
	}
	item := newItem(
		conversation, current.ID,
		[]store.ConversationMember{
			{ConversationID: conversation.ID, MemberType: store.ConversationMemberTypeUser, MemberID: current.ID},
			{ConversationID: conversation.ID, MemberType: store.ConversationMemberTypeUser, MemberID: target.ID},
		},
		map[string]store.User{current.ID: current, target.ID: target}, nil,
	)
	return OpenResult{Conversation: item, Created: created}, nil
}

func (s *Service) OpenDirectForUsers(ctx context.Context, current, target Identity) (Reference, bool, error) {
	conversation, created, err := s.getOrCreateDirect(s.db, actorUser(current), actorUser(target))
	if err != nil {
		return Reference{}, false, err
	}
	return newReference(conversation), created, nil
}

func (s *Service) getOrCreateDirect(db *gorm.DB, current, target store.User) (store.Conversation, bool, error) {
	lowID, highID := orderUserIDs(current.ID, target.ID)
	existing, err := findDirectByPair(db, lowID, highID)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return store.Conversation{}, false, err
	}
	now := s.now().UTC()
	conversation := store.Conversation{ID: uuid.NewString(), Kind: store.ConversationKindDirect, CreatedByUserID: current.ID, Status: store.ConversationStatusActive, PostingPolicy: store.ConversationPostingPolicyOpen, Visibility: store.ConversationVisibilityPrivate, CreatedAt: now, UpdatedAt: now}
	created := false
	err = db.Transaction(func(tx *gorm.DB) error {
		existing, err := findDirectByPair(tx, lowID, highID)
		if err == nil {
			conversation, created = existing, false
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Create(&conversation).Error; err != nil {
			return err
		}
		members := []store.ConversationMember{
			{ConversationID: conversation.ID, MemberType: store.ConversationMemberTypeUser, MemberID: current.ID, Role: store.ConversationMemberRoleOwner, JoinedAt: now, HistoryVisibleFromSeq: 1},
			{ConversationID: conversation.ID, MemberType: store.ConversationMemberTypeUser, MemberID: target.ID, Role: store.ConversationMemberRoleMember, JoinedAt: now, HistoryVisibleFromSeq: 1},
		}
		if err := tx.Create(&members).Error; err != nil {
			return err
		}
		if err := tx.Create(&store.DirectConversation{ConversationID: conversation.ID, UserLowID: lowID, UserHighID: highID, CreatedAt: now}).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	if err != nil {
		if isUniqueConstraintError(err) {
			if existing, findErr := findDirectByPair(db, lowID, highID); findErr == nil {
				return existing, false, nil
			}
		}
		return store.Conversation{}, false, err
	}
	return conversation, created, nil
}

func findDirectByPair(db *gorm.DB, lowID, highID string) (store.Conversation, error) {
	var direct store.DirectConversation
	if err := db.First(&direct, "user_low_id = ? AND user_high_id = ?", lowID, highID).Error; err != nil {
		return store.Conversation{}, err
	}
	var conversation store.Conversation
	if err := db.First(&conversation, "id = ?", direct.ConversationID).Error; err != nil {
		return store.Conversation{}, err
	}
	return conversation, nil
}

func orderUserIDs(first, second string) (string, string) {
	ids := []string{strings.TrimSpace(first), strings.TrimSpace(second)}
	sort.Strings(ids)
	return ids[0], ids[1]
}
