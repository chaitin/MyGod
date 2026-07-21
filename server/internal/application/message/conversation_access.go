package message

import (
	"errors"
	"time"

	"app/internal/application/conversationaccess"
	"app/internal/store"

	"gorm.io/gorm"
)

type userConversationAccess struct {
	Context     conversationaccess.Context
	Member      store.ConversationMember
	Participant *store.ConversationTopicParticipant
}

func loadUserConversationAccess(db *gorm.DB, conversationID, userID string, lock bool) (userConversationAccess, error) {
	access, err := conversationaccess.Load(db, conversationID, lock)
	if err != nil {
		return userConversationAccess{}, err
	}
	if access.Conversation.Status != store.ConversationStatusActive ||
		(access.ParentConversation != nil && access.ParentConversation.Status != store.ConversationStatusActive) {
		return userConversationAccess{}, errConversationAccessDenied
	}
	member, err := conversationaccess.RequireUserMember(db, access, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return userConversationAccess{}, errConversationAccessDenied
		}
		return userConversationAccess{}, err
	}
	if !conversationaccess.TopicSourceVisibleToMember(access, member) {
		return userConversationAccess{}, errConversationAccessDenied
	}
	result := userConversationAccess{Context: access, Member: member}
	if access.IsTopic() {
		var participant store.ConversationTopicParticipant
		query := db.Where(
			"conversation_id = ? AND participant_type = ? AND participant_id = ?",
			conversationID, store.ConversationMemberTypeUser, userID,
		).Limit(1).Find(&participant)
		if query.Error != nil {
			return userConversationAccess{}, query.Error
		}
		if query.RowsAffected > 0 {
			result.Participant = &participant
		}
	}
	return result, nil
}

func (value userConversationAccess) visibleFromSeq() int64 {
	if value.Context.IsTopic() && value.Participant == nil {
		return 1
	}
	visibleFromSeq := value.Member.HistoryVisibleFromSeq
	if value.Participant != nil {
		visibleFromSeq = value.Participant.HistoryVisibleFromSeq
	}
	if visibleFromSeq < 1 {
		return 1
	}
	return visibleFromSeq
}

func ensureUserConversationSendable(db *gorm.DB, value userConversationAccess, userID string, readSeq int64, now time.Time) error {
	if err := validateUserConversationSendable(db, value); err != nil {
		return err
	}
	if !value.Context.IsTopic() {
		return nil
	}
	return conversationaccess.EnsureTopicParticipant(
		db, value.Context, store.ConversationMemberTypeUser, userID,
		store.TopicParticipantReasonMessage, readSeq, 0, now,
	)
}

func validateUserConversationSendable(db *gorm.DB, value userConversationAccess) error {
	if err := ensureConversationSendable(db, value.Context.Conversation); err != nil {
		return err
	}
	if value.Context.ParentConversation != nil {
		if err := ensureConversationSendable(db, *value.Context.ParentConversation); err != nil {
			return err
		}
	}
	return validateUserDirectAppAccess(db, value)
}

func validateUserDirectAppAccess(db *gorm.DB, value userConversationAccess) error {
	if err := conversationaccess.RequireUserDirectAppAccess(db, value.Context, value.Member.MemberID); err != nil {
		if errors.Is(err, conversationaccess.ErrDirectAppAccessDenied) {
			return errAppDirectAccessDenied
		}
		return err
	}
	return nil
}

func validateAppConversationSendable(db *gorm.DB, value conversationaccess.Context, appID string) error {
	if err := ensureConversationSendable(db, value.Conversation); err != nil {
		return err
	}
	if value.ParentConversation != nil {
		if err := ensureConversationSendable(db, *value.ParentConversation); err != nil {
			return err
		}
	}
	if err := conversationaccess.RequireAppDirectUserAccess(db, value, appID); err != nil {
		if errors.Is(err, conversationaccess.ErrDirectAppAccessDenied) {
			return errAppDirectAccessDenied
		}
		return err
	}
	return nil
}

func advanceUserConversationReadSeq(db *gorm.DB, value conversationaccess.Context, userID string, seq int64, messageID *string, now time.Time) error {
	if value.IsTopic() {
		return conversationaccess.AdvanceTopicParticipantReadSeq(
			db, value.Conversation.ID, store.ConversationMemberTypeUser, userID, seq, messageID, now,
		)
	}
	return advanceConversationMemberReadSeq(db, value.Conversation.ID, userID, seq)
}

func loadConversationDeliveryUserIDs(db *gorm.DB, value conversationaccess.Context) ([]string, error) {
	return conversationaccess.ActiveUserIDs(db, value)
}

func requireAppConversationAccess(db *gorm.DB, conversationID, appID string, lock, requireParticipant bool) (conversationaccess.Context, store.ConversationMember, *store.ConversationTopicParticipant, error) {
	access, err := conversationaccess.Load(db, conversationID, lock)
	if err != nil {
		return conversationaccess.Context{}, store.ConversationMember{}, nil, err
	}
	if access.Conversation.Status != store.ConversationStatusActive ||
		(access.ParentConversation != nil && access.ParentConversation.Status != store.ConversationStatusActive) {
		return conversationaccess.Context{}, store.ConversationMember{}, nil, errConversationAccessDenied
	}
	member, err := conversationaccess.RequireAppMember(db, access, appID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return conversationaccess.Context{}, store.ConversationMember{}, nil, errConversationAccessDenied
		}
		return conversationaccess.Context{}, store.ConversationMember{}, nil, err
	}
	if !conversationaccess.TopicSourceVisibleToMember(access, member) {
		return conversationaccess.Context{}, store.ConversationMember{}, nil, errConversationAccessDenied
	}
	if !access.IsTopic() {
		return access, member, nil, nil
	}
	participant, err := conversationaccess.TopicParticipant(db, conversationID, store.ConversationMemberTypeApp, appID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) && !requireParticipant {
			return access, member, nil, nil
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return conversationaccess.Context{}, store.ConversationMember{}, nil, errConversationAccessDenied
		}
		return conversationaccess.Context{}, store.ConversationMember{}, nil, err
	}
	return access, member, &participant, nil
}
