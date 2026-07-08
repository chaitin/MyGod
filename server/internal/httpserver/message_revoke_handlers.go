package httpserver

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"app/internal/store"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	errMessageAlreadyRevoked    = errors.New("message already revoked")
	errMessageRevokeUnsupported = errors.New("message revoke unsupported")
	errMessageRevokeForbidden   = errors.New("message revoke forbidden")
)

type revokeConversationMessageResponse struct {
	Message       messageResponse `json:"message"`
	SystemMessage messageResponse `json:"system_message"`
}

// revokeConversationMessage godoc
//
// @Summary 撤回会话消息
// @Description 普通用户可以撤回自己的消息；群主和管理员可以撤回群内任意非系统消息。撤回后原消息只返回元信息，并创建一条系统消息。
// @Tags 客户端消息
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Param message_id path string true "消息 ID"
// @Success 200 {object} successEnvelope{data=revokeConversationMessageResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 409 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/{conversation_id}/messages/{message_id}/revoke [post]
func (s *Server) revokeConversationMessage(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	conversationID, err := normalizeMessageConversationID(c.Param("conversation_id"))
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}
	messageID, err := normalizeRequiredMessageID(c.Param("message_id"), "消息 ID")
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}

	message, systemMessage, memberUserIDs, err := s.revokeConversationMessageForUser(user, conversationID, messageID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return failure(c, http.StatusNotFound, "not_found", "消息不存在")
		}
		if errors.Is(err, errConversationAccessDenied) || errors.Is(err, errMessageRevokeForbidden) {
			return failure(c, http.StatusForbidden, "forbidden", "无权撤回消息")
		}
		if errors.Is(err, errMessageRevokeUnsupported) {
			return failure(c, http.StatusBadRequest, "invalid_request", "不能撤回该消息")
		}
		if errors.Is(err, errMessageAlreadyRevoked) {
			return failure(c, http.StatusConflict, "conflict", "消息已被撤回")
		}

		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	messageResponse, err := s.newMessageResponseForUser(c.Request().Context(), message, user.ID)
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	systemMessageResponse, err := s.newMessageResponseForUser(c.Request().Context(), systemMessage, user.ID)
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	s.sendRealtimeMessageUpdatedToUsers(c.Request().Context(), memberUserIDs, message)
	s.sendRealtimeMessageCreatedToUsers(c.Request().Context(), memberUserIDs, systemMessage)

	return success(c, http.StatusOK, revokeConversationMessageResponse{
		Message:       messageResponse,
		SystemMessage: systemMessageResponse,
	})
}

func (s *Server) revokeConversationMessageForUser(user store.User, conversationID string, messageID string) (store.Message, store.Message, []string, error) {
	var message store.Message
	var systemMessage store.Message
	memberUserIDs := []string{}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		var conversation store.Conversation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&conversation, "id = ?", conversationID).Error; err != nil {
			return err
		}
		if conversation.Status != store.ConversationStatusActive {
			return errConversationAccessDenied
		}

		var member store.ConversationMember
		if err := tx.First(
			&member,
			"conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL",
			conversationID,
			store.ConversationMemberTypeUser,
			user.ID,
		).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errConversationAccessDenied
			}
			return err
		}

		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			First(&message, "id = ? AND conversation_id = ? AND deleted_at IS NULL", messageID, conversationID).Error; err != nil {
			return err
		}
		if message.RevokedAt != nil {
			return errMessageAlreadyRevoked
		}
		if message.SenderType == store.MessageSenderTypeSystem {
			return errMessageRevokeUnsupported
		}
		if !canRevokeConversationMessage(user.ID, member, conversation, message) {
			return errMessageRevokeForbidden
		}

		now := time.Now().UTC()
		revokedByUserID := user.ID
		if err := tx.Model(&store.Message{}).
			Where("id = ?", message.ID).
			Updates(map[string]any{
				"revoked_at":         now,
				"revoked_by_user_id": revokedByUserID,
				"updated_at":         now,
			}).Error; err != nil {
			return err
		}
		message.RevokedAt = &now
		message.RevokedByUserID = &revokedByUserID
		message.UpdatedAt = now

		createdSystemMessage, err := createMessageRevokedSystemMessage(tx, &conversation, user, now)
		if err != nil {
			return err
		}
		systemMessage = createdSystemMessage

		if err := advanceConversationMemberReadSeq(tx, conversationID, user.ID, systemMessage.Seq); err != nil {
			return err
		}

		ids, err := loadActiveConversationUserIDs(tx, conversationID)
		if err != nil {
			return err
		}
		memberUserIDs = ids

		return nil
	})
	if err != nil {
		return store.Message{}, store.Message{}, nil, err
	}

	return message, systemMessage, memberUserIDs, nil
}

func normalizeRequiredMessageID(rawID string, fieldName string) (string, error) {
	id := strings.TrimSpace(rawID)
	if id == "" {
		return "", errors.New(fieldName + " 不能为空")
	}
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return "", errors.New(fieldName + " 格式错误")
	}

	return parsedID.String(), nil
}

func canRevokeConversationMessage(userID string, member store.ConversationMember, conversation store.Conversation, message store.Message) bool {
	if message.SenderType == store.MessageSenderTypeUser && message.SenderID != nil && *message.SenderID == userID {
		return true
	}
	if conversation.Kind != store.ConversationKindGroup {
		return false
	}

	return member.Role == store.ConversationMemberRoleOwner || member.Role == store.ConversationMemberRoleAdmin
}

func createMessageRevokedSystemMessage(db *gorm.DB, conversation *store.Conversation, actor store.User, now time.Time) (store.Message, error) {
	body, summary, err := newMessageRevokedSystemEventBody(actor)
	if err != nil {
		return store.Message{}, err
	}

	message := store.Message{
		ID:             uuid.NewString(),
		ConversationID: conversation.ID,
		Seq:            conversation.LastMessageSeq + 1,
		SenderType:     store.MessageSenderTypeSystem,
		Body:           body,
		Summary:        summary,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := db.Create(&message).Error; err != nil {
		return store.Message{}, err
	}

	if err := db.Model(&store.Conversation{}).
		Where("id = ?", conversation.ID).
		Updates(map[string]any{
			"last_message_at":      message.CreatedAt,
			"last_message_id":      message.ID,
			"last_message_seq":     message.Seq,
			"last_message_summary": message.Summary,
			"updated_at":           now,
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
