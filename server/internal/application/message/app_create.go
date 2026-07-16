package message

import (
	"context"
	"errors"
	"time"

	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Service) CreateAsApp(ctx context.Context, cmd CreateAsAppCommand) (CreateResult, error) {
	if cmd.Finalize == nil {
		return CreateResult{}, internalError(errors.New("message finalizer is required"))
	}
	var created bool
	var message store.Message
	memberUserIDs := []string{}
	mentionedUserIDs := []string{}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conversation store.Conversation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&conversation, "id = ?", cmd.ConversationID).Error; err != nil {
			return err
		}
		if conversation.Status != store.ConversationStatusActive || conversation.PostingPolicy != store.ConversationPostingPolicyOpen {
			return errConversationNotSendable
		}
		var member store.ConversationMember
		if err := tx.First(
			&member, "conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL",
			cmd.ConversationID, store.ConversationMemberTypeApp, cmd.AppID,
		).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errConversationAccessDenied
			}
			return err
		}
		existing, ok, err := findExistingMessageByClientMessageID(
			tx, cmd.ConversationID, store.MessageSenderTypeApp, cmd.AppID, cmd.ClientMessageID,
		)
		if err != nil {
			return err
		}
		if ok {
			message = existing
			return nil
		}
		finalBody, summary, err := cmd.Finalize(ctx, cmd.Body)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		message = store.Message{
			ID: uuid.NewString(), ConversationID: cmd.ConversationID, Seq: conversation.LastMessageSeq + 1,
			SenderType: store.MessageSenderTypeApp, SenderID: &cmd.AppID, ClientMessageID: &cmd.ClientMessageID,
			Body: finalBody, Summary: summary, CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.Create(&message).Error; err != nil {
			return err
		}
		if err := tx.Model(&store.Conversation{}).Where("id = ?", cmd.ConversationID).Updates(map[string]any{
			"last_message_at": message.CreatedAt, "last_message_id": message.ID,
			"last_message_seq": message.Seq, "last_message_summary": message.Summary, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		memberUserIDs, err = loadActiveConversationUserIDs(tx, cmd.ConversationID)
		if err != nil {
			return err
		}
		mentionedUserIDs, err = updateConversationMentionedSeq(tx, conversation.Kind, cmd.ConversationID, message.Seq, finalBody)
		if err != nil {
			return err
		}
		created = true
		return nil
	})
	if err != nil {
		return CreateResult{}, mapAppCreateError(err)
	}
	converted := newAppCreateResultMessage(message)
	if created && s.notifications != nil {
		s.notifications.PublishSharedMessageCreated(ctx, memberUserIDs, converted)
		s.notifications.PublishMembersMentioned(ctx, mentionedUserIDs, message.ConversationID, message.Seq)
	}
	return CreateResult{Created: created, Message: converted}, nil
}

func (s *Service) CreateDelegated(ctx context.Context, cmd CreateDelegatedCommand) (CreateResult, error) {
	if cmd.Finalize == nil {
		return CreateResult{}, internalError(errors.New("message finalizer is required"))
	}
	var created bool
	var message store.Message
	memberUserIDs := []string{}
	mentionedUserIDs := []string{}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conversation store.Conversation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&conversation, "id = ?", cmd.ConversationID).Error; err != nil {
			return err
		}
		if conversation.Status != store.ConversationStatusActive || conversation.PostingPolicy != store.ConversationPostingPolicyOpen {
			return errConversationNotSendable
		}
		var member store.ConversationMember
		if err := tx.First(
			&member, "conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL",
			cmd.ConversationID, store.ConversationMemberTypeUser, cmd.AccountID,
		).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errConversationAccessDenied
			}
			return err
		}
		existing, ok, err := findExistingMessageByClientMessageID(
			tx, cmd.ConversationID, store.MessageSenderTypeUser, cmd.AccountID, cmd.ClientMessageID,
		)
		if err != nil {
			return err
		}
		if ok {
			message = existing
			return advanceConversationMemberReadSeq(tx, cmd.ConversationID, cmd.AccountID, existing.Seq)
		}
		finalBody, summary, err := cmd.Finalize(ctx, cmd.Body)
		if err != nil {
			return err
		}
		delegatedByType := cmd.DelegatedBy.Type
		delegatedByID := cmd.DelegatedBy.ID
		now := time.Now().UTC()
		message = store.Message{
			ID: uuid.NewString(), ConversationID: cmd.ConversationID, Seq: conversation.LastMessageSeq + 1,
			SenderType: store.MessageSenderTypeUser, SenderID: &cmd.AccountID, ClientMessageID: &cmd.ClientMessageID,
			DelegatedByType: &delegatedByType, DelegatedByID: &delegatedByID, DelegatedByName: cmd.DelegatedBy.Name,
			Body: finalBody, Summary: summary, CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.Create(&message).Error; err != nil {
			return err
		}
		if err := tx.Model(&store.Conversation{}).Where("id = ?", cmd.ConversationID).Updates(map[string]any{
			"last_message_at": message.CreatedAt, "last_message_id": message.ID,
			"last_message_seq": message.Seq, "last_message_summary": message.Summary, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if err := advanceConversationMemberReadSeq(tx, cmd.ConversationID, cmd.AccountID, message.Seq); err != nil {
			return err
		}
		mentionedUserIDs, err = updateConversationMentionedSeq(tx, conversation.Kind, cmd.ConversationID, message.Seq, finalBody)
		if err != nil {
			return err
		}
		memberUserIDs, err = loadActiveConversationUserIDs(tx, cmd.ConversationID)
		if err != nil {
			return err
		}
		created = true
		return nil
	})
	if err != nil {
		return CreateResult{}, mapAppCreateError(err)
	}
	converted := newAppCreateResultMessage(message)
	if created && s.notifications != nil {
		s.notifications.PublishSharedMessageCreated(ctx, memberUserIDs, converted)
		s.notifications.PublishMembersMentioned(ctx, mentionedUserIDs, message.ConversationID, message.Seq)
	}
	return CreateResult{Created: created, Message: converted}, nil
}

// App request responses historically return the stored body even when an
// idempotent retry finds a message that has since been revoked.
func newAppCreateResultMessage(message store.Message) Message {
	converted := newMessage(message)
	converted.Body = message.Body
	return converted
}

func mapAppCreateError(err error) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return NotFoundError("会话不存在", err)
	case errors.Is(err, errConversationAccessDenied):
		return forbidden("无权访问会话", err)
	case errors.Is(err, errConversationNotSendable):
		return forbidden("当前会话不能发送消息", err)
	default:
		return err
	}
}
