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
	"gorm.io/gorm/clause"
)

var (
	errMessageAlreadyRevoked    = errors.New("message already revoked")
	errMessageRevokeUnsupported = errors.New("message revoke unsupported")
	errMessageRevokeForbidden   = errors.New("message revoke forbidden")
)

type messageRevokedSystemEventBody struct {
	Actor struct {
		DisplayName string `json:"display_name"`
		ID          string `json:"id"`
		Type        string `json:"type,omitempty"`
	} `json:"actor"`
	Event string `json:"event"`
	Type  string `json:"type"`
}

func (s *Service) Revoke(ctx context.Context, cmd RevokeCommand) (RevokeResult, error) {
	conversationID, err := normalizeRequiredUUID(cmd.ConversationID, "会话 ID")
	if err != nil {
		return RevokeResult{}, InvalidRequestError(err.Error(), err)
	}
	messageID, err := normalizeRequiredUUID(cmd.MessageID, "消息 ID")
	if err != nil {
		return RevokeResult{}, InvalidRequestError(err.Error(), err)
	}

	message, systemMessage, memberUserIDs, reactionVersion, err := s.revokeForUser(ctx, cmd.AccountID, conversationID, messageID)
	if err != nil {
		return RevokeResult{}, mapRevokeError(err)
	}
	db := s.db.WithContext(ctx)
	response, err := newMessageForUser(db, message, cmd.AccountID)
	if err != nil {
		return RevokeResult{}, internalError(err)
	}
	response.ReactionVersion = reactionVersion
	response.Reactions = []ReactionSummary{}
	systemResponse, err := newMessageForUser(db, systemMessage, cmd.AccountID)
	if err != nil {
		return RevokeResult{}, internalError(err)
	}
	responseWithTopic := []Message{response}
	if err := attachMessageTopics(db, responseWithTopic); err != nil {
		return RevokeResult{}, internalError(err)
	}
	response = responseWithTopic[0]

	if s.notifications != nil {
		updated := make([]Delivery, 0, len(memberUserIDs))
		created := make([]Delivery, 0, len(memberUserIDs))
		for _, userID := range memberUserIDs {
			updatedMessage, viewErr := newMessageForUser(db, message, userID)
			if viewErr != nil {
				updatedMessage = newMessage(message)
			}
			updatedMessage.ReactionVersion = reactionVersion
			updatedMessage.Reactions = []ReactionSummary{}
			updatedWithTopic := []Message{updatedMessage}
			if topicErr := attachMessageTopics(db, updatedWithTopic); topicErr == nil {
				updatedMessage = updatedWithTopic[0]
			}
			createdMessage, viewErr := newMessageForUser(db, systemMessage, userID)
			if viewErr != nil {
				createdMessage = newMessage(systemMessage)
			}
			updated = append(updated, Delivery{Message: updatedMessage, UserID: userID})
			created = append(created, Delivery{Message: createdMessage, UserID: userID})
		}
		s.notifications.PublishMessageUpdated(ctx, updated)
		s.notifications.PublishMessageCreated(ctx, created)
	}

	return RevokeResult{Message: response, SystemMessage: systemResponse}, nil
}

func (s *Service) revokeForUser(ctx context.Context, userID, conversationID, messageID string) (store.Message, store.Message, []string, int64, error) {
	var message store.Message
	var systemMessage store.Message
	memberUserIDs := []string{}
	var reactionVersion int64

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		access, err := loadUserConversationAccess(tx, conversationID, userID, true)
		if err != nil {
			return err
		}
		conversation := access.Context.Conversation
		if err := validateUserConversationSendable(tx, access); err != nil {
			return err
		}

		messagePartitionYear := 0
		if store.MessagePartitioningEnabled(tx) {
			var registry store.MessageRegistry
			if err := applyOnlineStoredMessageWindow(tx.Clauses(clause.Locking{Strength: "UPDATE"})).
				First(&registry, "id = ? AND conversation_id = ? AND deleted_at IS NULL", messageID, conversationID).Error; err != nil {
				return err
			}
			messagePartitionYear = int(registry.PartitionYear)
			scope, err := store.ScopeMessagePartition(ctx, tx, messagePartitionYear)
			if err != nil {
				return err
			}
			if err := scope.Clauses(clause.Locking{Strength: "UPDATE"}).
				Take(&message, "id = ? AND conversation_id = ? AND deleted_at IS NULL", messageID, conversationID).Error; err != nil {
				return err
			}
		} else if err := applyOnlineStoredMessageWindow(tx.Clauses(clause.Locking{Strength: "UPDATE"})).
			First(&message, "id = ? AND conversation_id = ? AND deleted_at IS NULL", messageID, conversationID).Error; err != nil {
			return err
		}
		if message.RevokedAt != nil {
			return errMessageAlreadyRevoked
		}
		if message.SenderType == store.MessageSenderTypeSystem {
			return errMessageRevokeUnsupported
		}
		if !canRevokeMessage(userID, access.Member, access.Context.EffectiveConversation(), message) {
			return errMessageRevokeForbidden
		}

		var actor store.User
		if err := tx.First(&actor, "id = ?", userID).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		revokedByUserID := userID
		messageUpdates := map[string]any{
			"revoked_at": now, "revoked_by_user_id": revokedByUserID, "updated_at": now,
		}
		messageUpdate := tx.Model(&store.Message{}).Where("conversation_id = ? AND id = ?", conversationID, message.ID)
		if messagePartitionYear != 0 {
			scope, err := store.ScopeMessagePartition(ctx, tx, messagePartitionYear)
			if err != nil {
				return err
			}
			messageUpdate = scope.Where("conversation_id = ? AND id = ?", conversationID, message.ID)
		}
		if err := messageUpdate.Updates(messageUpdates).Error; err != nil {
			return err
		}
		message.RevokedAt = &now
		message.RevokedByUserID = &revokedByUserID
		message.UpdatedAt = now
		reactionState, err := lockMessageReactionState(tx, message.ID, now)
		if err != nil {
			return err
		}
		if err := tx.Where("message_id = ?", message.ID).Delete(&store.MessageReaction{}).Error; err != nil {
			return err
		}
		if err := incrementMessageReactionVersion(tx, &reactionState, now); err != nil {
			return err
		}
		reactionVersion = reactionState.Version

		createdSystemMessage, err := createMessageRevokedSystemMessage(tx, &conversation, actor, now)
		if err != nil {
			return err
		}
		systemMessage = createdSystemMessage
		if !access.Context.IsTopic() || access.Participant != nil {
			if err := advanceUserConversationReadSeq(tx, access.Context, userID, systemMessage.Seq, &systemMessage.ID, now); err != nil {
				return err
			}
		}
		memberUserIDs, err = loadConversationDeliveryUserIDs(tx, access.Context)
		return err
	})
	return message, systemMessage, memberUserIDs, reactionVersion, err
}

func canRevokeMessage(userID string, member store.ConversationMember, conversation store.Conversation, message store.Message) bool {
	if message.SenderType == store.MessageSenderTypeUser && message.SenderID != nil && *message.SenderID == userID {
		return true
	}
	if conversation.Kind != store.ConversationKindGroup {
		return false
	}
	return member.Role == store.ConversationMemberRoleOwner || member.Role == store.ConversationMemberRoleAdmin
}

func createMessageRevokedSystemMessage(db *gorm.DB, conversation *store.Conversation, actor store.User, now time.Time) (store.Message, error) {
	displayName := strings.TrimSpace(actor.Nickname)
	if displayName == "" {
		displayName = strings.TrimSpace(actor.Name)
	}
	bodyValue := messageRevokedSystemEventBody{Event: "message_revoked", Type: "system_event"}
	bodyValue.Actor.DisplayName = displayName
	bodyValue.Actor.ID = actor.ID
	body, err := json.Marshal(bodyValue)
	if err != nil {
		return store.Message{}, err
	}
	message := store.Message{
		ID: uuid.NewString(), ConversationID: conversation.ID, Seq: conversation.LastMessageSeq + 1,
		SenderType: store.MessageSenderTypeSystem, Body: body, Summary: displayName + " 撤回了一条消息",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&message).Error; err != nil {
		return store.Message{}, err
	}
	if err := db.Model(&store.Conversation{}).Where("id = ?", conversation.ID).Updates(map[string]any{
		"last_message_at": message.CreatedAt, "last_message_id": message.ID,
		"last_message_seq": message.Seq, "last_message_summary": message.Summary, "updated_at": now,
	}).Error; err != nil {
		return store.Message{}, err
	}
	lastMessageAt := message.CreatedAt
	conversation.LastMessageAt = &lastMessageAt
	conversation.LastMessageID = &message.ID
	conversation.LastMessageSeq = message.Seq
	conversation.LastMessageSummary = message.Summary
	conversation.UpdatedAt = now
	return message, nil
}

func mapRevokeError(err error) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return NotFoundError("消息不存在", err)
	case errors.Is(err, errConversationAccessDenied), errors.Is(err, errMessageRevokeForbidden):
		return forbidden("无权撤回消息", err)
	case errors.Is(err, errConversationNotSendable):
		return forbidden("当前会话不能撤回消息", err)
	case errors.Is(err, errAppDirectAccessDenied):
		return forbidden("你当前无权直接使用此应用", err)
	case errors.Is(err, errMessageRevokeUnsupported):
		return InvalidRequestError("不能撤回该消息", err)
	case errors.Is(err, errMessageAlreadyRevoked):
		return conflict("消息已被撤回", err)
	default:
		return internalError(err)
	}
}
