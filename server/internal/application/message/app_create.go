package message

import (
	"context"
	"errors"
	"time"

	appapp "app/internal/application/app"
	"app/internal/application/conversationaccess"
	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (s *Service) CreateAsApp(ctx context.Context, cmd CreateAsAppCommand) (CreateResult, error) {
	if cmd.Finalize == nil {
		return CreateResult{}, internalError(errors.New("message finalizer is required"))
	}
	var created bool
	var message store.Message
	memberUserIDs := []string{}
	mentionedUserIDs := []string{}
	choiceUserIDs := []string{}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := appapp.LockUsableApp(tx, cmd.AppID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errConversationAccessDenied
			}
			return err
		}
		access, _, _, err := requireAppConversationAccess(tx, cmd.ConversationID, cmd.AppID, true, false)
		if err != nil {
			return err
		}
		conversation := access.Conversation
		if err := validateAppConversationSendable(tx, access, cmd.AppID); err != nil {
			return err
		}
		if err := conversationaccess.EnsureTopicParticipant(
			tx, access, store.ConversationMemberTypeApp, cmd.AppID,
			store.TopicParticipantReasonMessage, 0, 0, time.Now().UTC(),
		); err != nil {
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
		mentionedUserIDs, err = updateConversationMentionedSeq(tx, access, message.Seq, finalBody, now)
		if err != nil {
			return err
		}
		memberUserIDs, err = loadConversationDeliveryUserIDs(tx, access)
		if err != nil {
			return err
		}
		choiceUserIDs, err = updateConversationChoiceSeq(tx, access, message.Seq, finalBody, memberUserIDs, now)
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
		s.notifications.PublishMembersChoiceReceived(ctx, choiceUserIDs, message.ConversationID, message.Seq)
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
	choiceUserIDs := []string{}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		access, err := loadUserConversationAccess(tx, cmd.ConversationID, cmd.AccountID, true)
		if err != nil {
			return err
		}
		conversation := access.Context.Conversation
		if err := ensureUserConversationSendable(tx, access, cmd.AccountID, 0, time.Now().UTC()); err != nil {
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
			return advanceUserConversationReadSeq(tx, access.Context, cmd.AccountID, existing.Seq, &existing.ID, time.Now().UTC())
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
		if err := advanceUserConversationReadSeq(tx, access.Context, cmd.AccountID, message.Seq, &message.ID, now); err != nil {
			return err
		}
		mentionedUserIDs, err = updateConversationMentionedSeq(tx, access.Context, message.Seq, finalBody, now)
		if err != nil {
			return err
		}
		memberUserIDs, err = loadConversationDeliveryUserIDs(tx, access.Context)
		if err != nil {
			return err
		}
		choiceUserIDs, err = updateConversationChoiceSeq(tx, access.Context, message.Seq, finalBody, memberUserIDs, now)
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
		s.notifications.PublishMembersChoiceReceived(ctx, choiceUserIDs, message.ConversationID, message.Seq)
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
	case errors.Is(err, errAppDirectAccessDenied):
		return forbidden("对方当前无权直接使用此应用", err)
	default:
		return err
	}
}
