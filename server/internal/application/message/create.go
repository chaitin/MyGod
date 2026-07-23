package message

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"app/internal/store"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (s *Service) Create(ctx context.Context, cmd CreateCommand) (CreateResult, error) {
	conversationID, err := normalizeRequiredUUID(cmd.ConversationID, "会话 ID")
	if err != nil {
		return CreateResult{}, InvalidRequestError(err.Error(), err)
	}
	clientMessageID, err := normalizeClientMessageID(cmd.ClientMessageID)
	if err != nil {
		return CreateResult{}, InvalidRequestError(err.Error(), err)
	}
	replyToMessageID, err := normalizeOptionalUUID(cmd.ReplyToMessageID, "引用消息 ID")
	if err != nil {
		return CreateResult{}, InvalidRequestError(err.Error(), err)
	}
	if s.bodies == nil {
		return CreateResult{}, internalError(errors.New("message body processor is required"))
	}
	preparedBody, err := s.bodies.Prepare(ctx, cmd.AccountID, cmd.Body)
	if err != nil {
		var messageErr *Error
		if errors.As(err, &messageErr) {
			return CreateResult{}, err
		}
		return CreateResult{}, internalError(err)
	}

	return s.createFinalizedMessage(
		ctx, cmd.AccountID, conversationID, clientMessageID, replyToMessageID, preparedBody,
		s.bodies.Finalize,
	)
}

func (s *Service) CreatePrepared(ctx context.Context, cmd CreatePreparedCommand) (CreateResult, error) {
	conversationID, err := normalizeRequiredUUID(cmd.ConversationID, "会话 ID")
	if err != nil {
		return CreateResult{}, InvalidRequestError(err.Error(), err)
	}
	clientMessageID, err := normalizeClientMessageID(cmd.ClientMessageID)
	if err != nil {
		return CreateResult{}, InvalidRequestError(err.Error(), err)
	}
	replyToMessageID, err := normalizeOptionalUUID(cmd.ReplyToMessageID, "引用消息 ID")
	if err != nil {
		return CreateResult{}, InvalidRequestError(err.Error(), err)
	}
	return s.createFinalizedMessage(
		ctx, cmd.AccountID, conversationID, clientMessageID, replyToMessageID, cmd.Body,
		func(_ context.Context, body json.RawMessage) (json.RawMessage, string, error) {
			return body, cmd.Summary, nil
		},
	)
}

func (s *Service) PrepareUpload(ctx context.Context, cmd PrepareUploadCommand) (PrepareUploadResult, error) {
	conversationID, err := normalizeRequiredUUID(cmd.ConversationID, "会话 ID")
	if err != nil {
		return PrepareUploadResult{}, InvalidRequestError(err.Error(), err)
	}
	clientMessageID, err := normalizeClientMessageID(cmd.ClientMessageID)
	if err != nil {
		return PrepareUploadResult{}, InvalidRequestError(err.Error(), err)
	}
	replyToMessageID, err := normalizeOptionalUUID(cmd.ReplyToMessageID, "引用消息 ID")
	if err != nil {
		return PrepareUploadResult{}, InvalidRequestError(err.Error(), err)
	}
	db := s.db.WithContext(ctx)
	access, err := loadUserConversationAccess(db, conversationID, cmd.AccountID, false)
	if err != nil {
		return PrepareUploadResult{}, mapCreateError(err)
	}
	if err := validateUserConversationSendable(db, access); err != nil {
		return PrepareUploadResult{}, mapCreateError(err)
	}
	existing, ok, err := findExistingMessageByClientMessageID(
		db, conversationID, store.MessageSenderTypeUser, cmd.AccountID, clientMessageID,
	)
	if err != nil {
		return PrepareUploadResult{}, internalError(err)
	}
	if ok {
		if err := advanceUserConversationReadSeq(db, access.Context, cmd.AccountID, existing.Seq, &existing.ID, time.Now().UTC()); err != nil {
			return PrepareUploadResult{}, internalError(err)
		}
		converted, err := newMessageForUser(db, existing, cmd.AccountID)
		if err != nil {
			return PrepareUploadResult{}, internalError(err)
		}
		return PrepareUploadResult{Existing: &converted}, nil
	}
	if err := validateReplyToMessage(db, conversationID, access.visibleFromSeq(), replyToMessageID); err != nil {
		return PrepareUploadResult{}, mapCreateError(err)
	}
	return PrepareUploadResult{}, nil
}

type finalizeBodyFunc func(context.Context, json.RawMessage) (json.RawMessage, string, error)

func (s *Service) createFinalizedMessage(
	ctx context.Context,
	accountID string,
	conversationID string,
	clientMessageID string,
	replyToMessageID *string,
	body json.RawMessage,
	finalize finalizeBodyFunc,
) (CreateResult, error) {
	stored, created, memberUserIDs, mentionedUserIDs, choiceUserIDs, events, lockHeld, err := s.createUserMessage(
		ctx, accountID, conversationID, clientMessageID, replyToMessageID, body, finalize,
	)
	if lockHeld {
		defer s.appEventLocker.Unlock()
	}
	if err != nil {
		return CreateResult{}, mapCreateError(err)
	}
	if created {
		message := newMessage(stored)
		if s.notifications != nil {
			deliveries := make([]Delivery, 0, len(memberUserIDs))
			for _, userID := range memberUserIDs {
				converted, viewErr := newMessageForUser(s.db.WithContext(ctx), stored, userID)
				if viewErr != nil {
					converted = message
				}
				deliveries = append(deliveries, Delivery{Message: converted, UserID: userID})
			}
			s.notifications.PublishMessageCreated(ctx, deliveries)
			s.notifications.PublishMembersMentioned(ctx, mentionedUserIDs, stored.ConversationID, stored.Seq)
			s.notifications.PublishMembersChoiceReceived(ctx, choiceUserIDs, stored.ConversationID, stored.Seq)
		}
		if s.afterUserMessageCommit != nil {
			s.afterUserMessageCommit(message)
		}
		if s.appEvents != nil {
			s.appEvents.DeliverAppEvents(ctx, events)
		}
	}

	response, err := newMessageForUser(s.db.WithContext(ctx), stored, accountID)
	if err != nil {
		return CreateResult{}, internalError(err)
	}
	return CreateResult{Created: created, Message: response}, nil
}

func (s *Service) createUserMessage(
	ctx context.Context,
	userID string,
	conversationID string,
	clientMessageID string,
	replyToMessageID *string,
	body json.RawMessage,
	finalize finalizeBodyFunc,
) (store.Message, bool, []string, []string, []string, []AppEvent, bool, error) {
	var created bool
	var message store.Message
	var events []AppEvent
	lockHeld := false
	memberUserIDs := []string{}
	mentionedUserIDs := []string{}
	choiceUserIDs := []string{}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		access, err := loadUserConversationAccess(tx, conversationID, userID, true)
		if err != nil {
			return err
		}
		conversation := access.Context.Conversation
		if err := ensureUserConversationSendable(tx, access, userID, 0, time.Now().UTC()); err != nil {
			return err
		}
		existing, ok, err := findExistingMessageByClientMessageID(
			tx, conversationID, store.MessageSenderTypeUser, userID, clientMessageID,
		)
		if err != nil {
			return err
		}
		if ok {
			message = existing
			if err := advanceUserConversationReadSeq(tx, access.Context, userID, existing.Seq, &existing.ID, time.Now().UTC()); err != nil {
				return err
			}
			return nil
		}
		if err := validateReplyToMessage(tx, conversationID, access.visibleFromSeq(), replyToMessageID); err != nil {
			return err
		}
		finalBody, summary, err := finalize(ctx, body)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		message = store.Message{
			ID: uuid.NewString(), ConversationID: conversationID, Seq: conversation.LastMessageSeq + 1,
			SenderType: store.MessageSenderTypeUser, SenderID: &userID, ClientMessageID: &clientMessageID,
			ReplyToMessageID: replyToMessageID, Body: finalBody, Summary: summary,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.Create(&message).Error; err != nil {
			return err
		}
		if err := tx.Model(&store.Conversation{}).Where("id = ?", conversationID).Updates(map[string]any{
			"last_message_at": message.CreatedAt, "last_message_id": message.ID,
			"last_message_seq": message.Seq, "last_message_summary": message.Summary, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if err := advanceUserConversationReadSeq(tx, access.Context, userID, message.Seq, &message.ID, now); err != nil {
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
		var sender store.User
		if err := tx.First(&sender, "id = ?", userID).Error; err != nil {
			return err
		}
		converted := newMessage(message)
		if s.beforeAppEventLock != nil {
			s.beforeAppEventLock(converted)
		}
		if s.appEventLocker != nil {
			s.appEventLocker.Lock()
			lockHeld = true
		}
		events, err = createAppMessageEventOutbox(tx, access.Context, sender, message)
		return err
	})
	return message, created, memberUserIDs, mentionedUserIDs, choiceUserIDs, events, lockHeld, err
}

func ensureConversationSendable(db *gorm.DB, conversation store.Conversation) error {
	if conversation.Status != store.ConversationStatusActive || conversation.PostingPolicy != store.ConversationPostingPolicyOpen {
		return errConversationNotSendable
	}
	if conversation.Kind != store.ConversationKindApp {
		return nil
	}
	var candidateIDs []string
	if err := db.Model(&store.ConversationMember{}).Where(
		"conversation_id = ? AND member_type = ? AND left_at IS NULL",
		conversation.ID, store.ConversationMemberTypeApp,
	).Pluck("member_id", &candidateIDs).Error; err != nil {
		return err
	}
	activeIDs, err := lockAndFilterActiveConversationApps(db, conversation.ID, candidateIDs)
	if err != nil {
		return err
	}
	if len(activeIDs) == 0 {
		return errConversationNotSendable
	}
	return nil
}

func findExistingMessageByClientMessageID(db *gorm.DB, conversationID, senderType, senderID, clientMessageID string) (store.Message, bool, error) {
	if store.MessagePartitioningEnabled(db) {
		var registry store.MessageRegistry
		result := db.Model(&store.MessageRegistry{}).Order("id ASC").Limit(1).Find(
			&registry,
			"conversation_id = ? AND sender_type = ? AND sender_id = ? AND client_message_id = ?",
			conversationID, senderType, senderID, clientMessageID,
		)
		if result.Error != nil {
			return store.Message{}, false, result.Error
		}
		if result.RowsAffected == 0 {
			return store.Message{}, false, nil
		}
		message, err := store.LoadMessageByRegistry(messageStorageContext(db), db, registry)
		return message, err == nil, err
	}
	var value store.Message
	result := db.Order("id ASC").Limit(1).Find(
		&value,
		"conversation_id = ? AND sender_type = ? AND sender_id = ? AND client_message_id = ?",
		conversationID, senderType, senderID, clientMessageID,
	)
	if result.Error != nil {
		return store.Message{}, false, result.Error
	}
	return value, result.RowsAffected > 0, nil
}

func validateReplyToMessage(db *gorm.DB, conversationID string, visibleFromSeq int64, replyToMessageID *string) error {
	if replyToMessageID == nil {
		return nil
	}
	if visibleFromSeq < 1 {
		visibleFromSeq = 1
	}
	model := any(&store.Message{})
	if store.MessagePartitioningEnabled(db) {
		model = &store.MessageRegistry{}
	}
	var value struct{ ID string }
	err := applyOnlineStoredMessageWindow(db.Model(model)).Select("id").Where(
		"id = ? AND conversation_id = ? AND deleted_at IS NULL AND seq >= ?",
		*replyToMessageID, conversationID, visibleFromSeq,
	).Limit(1).Take(&value).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errReplyToMessageInvalid
	}
	return err
}

func advanceConversationMemberReadSeq(db *gorm.DB, conversationID, userID string, seq int64) error {
	return db.Model(&store.ConversationMember{}).
		Where("conversation_id = ? AND member_type = ? AND member_id = ?", conversationID, store.ConversationMemberTypeUser, userID).
		Update("last_read_seq", gorm.Expr("CASE WHEN last_read_seq > ? THEN last_read_seq ELSE ? END", seq, seq)).Error
}

func loadActiveConversationUserIDs(db *gorm.DB, conversationID string) ([]string, error) {
	var members []store.ConversationMember
	if err := db.Where(
		"conversation_id = ? AND member_type = ? AND left_at IS NULL",
		conversationID, store.ConversationMemberTypeUser,
	).Order("joined_at ASC").Find(&members).Error; err != nil {
		return nil, err
	}
	result := make([]string, 0, len(members))
	for _, member := range members {
		result = append(result, member.MemberID)
	}
	return result, nil
}

func normalizeRequiredUUID(raw, field string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errors.New(field + " 不能为空")
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return "", errors.New(field + " 格式错误")
	}
	return parsed.String(), nil
}

func normalizeOptionalUUID(raw, field string) (*string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return nil, errors.New(field + " 格式错误")
	}
	normalized := parsed.String()
	return &normalized, nil
}

func normalizeClientMessageID(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errors.New("客户端消息 ID 不能为空")
	}
	if len([]rune(value)) > MaxClientMessageID {
		return "", errors.New("客户端消息 ID 不能超过 128 个字符")
	}
	return value, nil
}

func mapCreateError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return NotFoundError("会话不存在", err)
	}
	if errors.Is(err, errConversationAccessDenied) {
		return forbidden("无权访问会话", err)
	}
	if errors.Is(err, errConversationNotSendable) {
		return forbidden("当前会话不能发送消息", err)
	}
	if errors.Is(err, errAppDirectAccessDenied) {
		return forbidden("你当前无权直接使用此应用", err)
	}
	if errors.Is(err, errReplyToMessageInvalid) {
		return InvalidRequestError("引用消息无效", err)
	}
	return internalError(err)
}
