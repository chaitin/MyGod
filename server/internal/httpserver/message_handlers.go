package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	stdhtml "html"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"app/internal/realtime"
	"app/internal/store"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extensionast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
	nethtml "golang.org/x/net/html"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	defaultMessageHistoryLimit  = 20
	maxMessageHistoryLimit      = 20
	maxClientMessageIDLength    = 128
	maxLinkMessageURLLength     = 2048
	maxLinkPreviewReadBytes     = 1024
	maxTextMessageContentLength = 5000
	linkPreviewFetchTimeout     = 2 * time.Second
	linkPreviewMaxRedirects     = 3
	messageTypeLink             = "link"
	messageTypeMarkdown         = "markdown"
	messageTypeText             = "text"
)

var (
	errConversationAccessDenied = errors.New("conversation access denied")
	errConversationNotSendable  = errors.New("conversation not sendable")
	errReplyToMessageInvalid    = errors.New("reply_to_message_id invalid")
)

type createMessageRequest struct {
	ClientMessageID  string          `json:"client_message_id" example:"9c08f2dd-0af6-4e99-b486-2f0c841822be"`
	ReplyToMessageID string          `json:"reply_to_message_id,omitempty" example:"9c08f2dd-0af6-4e99-b486-2f0c841822be"`
	Body             json.RawMessage `json:"body" swaggertype:"object"`
}

type messageSenderResponse struct {
	ID   string `json:"id,omitempty" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	Type string `json:"type" example:"user"`
}

type messageDelegatedByResponse struct {
	ID   string `json:"id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	Name string `json:"name" example:"女菩萨"`
	Type string `json:"type" example:"app"`
}

type messageReplyToSenderResponse struct {
	ID   string `json:"id,omitempty" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	Name string `json:"name" example:"Alice"`
	Type string `json:"type" example:"user"`
}

type messageReplyToResponse struct {
	ID      string                       `json:"id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	Sender  messageReplyToSenderResponse `json:"sender"`
	Seq     int64                        `json:"seq" example:"12"`
	Summary string                       `json:"summary" example:"上一条消息摘要"`
}

type messageResponse struct {
	ClientMessageID  string                      `json:"client_message_id" example:"9c08f2dd-0af6-4e99-b486-2f0c841822be"`
	Body             json.RawMessage             `json:"body,omitempty" swaggertype:"object"`
	ConversationID   string                      `json:"conversation_id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	CreatedAt        time.Time                   `json:"created_at" format:"date-time"`
	DelegatedBy      *messageDelegatedByResponse `json:"delegated_by,omitempty"`
	ID               string                      `json:"id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	ReplyToMessageID string                      `json:"reply_to_message_id,omitempty" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	ReplyTo          *messageReplyToResponse     `json:"reply_to,omitempty"`
	RevokedAt        *time.Time                  `json:"revoked_at,omitempty" format:"date-time"`
	RevokedByUserID  string                      `json:"revoked_by_user_id,omitempty" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	Sender           messageSenderResponse       `json:"sender"`
	Seq              int64                       `json:"seq" example:"13"`
}

type createMessageResponse struct {
	Message messageResponse `json:"message"`
}

type listMessagesPageResponse struct {
	HasMoreAfter  bool  `json:"has_more_after" example:"false"`
	HasMoreBefore bool  `json:"has_more_before" example:"true"`
	Limit         int   `json:"limit" example:"20"`
	NewestSeq     int64 `json:"newest_seq" example:"120"`
	OldestSeq     int64 `json:"oldest_seq" example:"101"`
}

type listConversationMessagesResponse struct {
	Messages []messageResponse        `json:"messages"`
	Page     listMessagesPageResponse `json:"page"`
}

type listConversationMessagesQuery struct {
	AfterSeq  *int64
	BeforeSeq *int64
	Limit     int
}

type textMessageBody struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

type markdownMessageBody struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

type linkMessageBody struct {
	Type  string `json:"type"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

type messageBodyEnvelope struct {
	Type string `json:"type"`
}

type messageBodyHandler interface {
	Type() string
	Validate(body json.RawMessage) error
	Normalize(ctx context.Context, body json.RawMessage) (json.RawMessage, error)
	Summary(body json.RawMessage) (string, error)
}

type messageBodyFinalizer interface {
	Finalize(ctx context.Context, body json.RawMessage) (json.RawMessage, error)
}

type finalizeMessageBodyFunc func(ctx context.Context, body json.RawMessage) (json.RawMessage, string, error)

type createMessageMetadata struct {
	DelegatedByType  *string
	DelegatedByID    *string
	DelegatedByName  string
	ReplyToMessageID *string
}

type textMessageBodyHandler struct{}
type markdownMessageBodyHandler struct{}
type linkMessageBodyHandler struct{}

var messageBodyHandlers = map[string]messageBodyHandler{
	messageTypeLink:     linkMessageBodyHandler{},
	messageTypeMarkdown: markdownMessageBodyHandler{},
	messageTypeText:     textMessageBodyHandler{},
}

var markdownParser = goldmark.New(goldmark.WithExtensions(extension.Table, extension.Strikethrough))
var linkPreviewHTTPClient = newLinkPreviewHTTPClient()
var fetchLinkPreviewTitle = fetchLinkPreviewTitleHTTP

// listConversationMessages godoc
//
// @Summary 拉取会话历史消息
// @Description 普通用户拉取自己参与的 active 会话消息。默认返回最近 20 条，支持 before_seq/after_seq 游标。
// @Tags 客户端消息
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Param before_seq query int false "拉取此 seq 之前的更早消息"
// @Param after_seq query int false "拉取此 seq 之后的更新消息"
// @Param limit query int false "返回数量，默认 20，最大 20"
// @Success 200 {object} successEnvelope{data=listConversationMessagesResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/{conversation_id}/messages [get]
func (s *Server) listConversationMessages(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	conversationID, err := normalizeMessageConversationID(c.Param("conversation_id"))
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}

	query, err := normalizeListConversationMessagesQuery(c.QueryParam("before_seq"), c.QueryParam("after_seq"), c.QueryParam("limit"))
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}

	messages, page, err := s.listUserConversationMessages(user.ID, conversationID, query)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return failure(c, http.StatusNotFound, "not_found", "会话不存在")
		}
		if errors.Is(err, errConversationAccessDenied) {
			return failure(c, http.StatusForbidden, "forbidden", "无权访问会话")
		}

		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	responses := make([]messageResponse, 0, len(messages))
	for _, message := range messages {
		response, err := s.newMessageResponseForUser(c.Request().Context(), message, user.ID)
		if err != nil {
			return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
		}
		responses = append(responses, response)
	}

	return success(c, http.StatusOK, listConversationMessagesResponse{
		Messages: responses,
		Page:     page,
	})
}

// createConversationMessage godoc
//
// @Summary 发送消息
// @Description 普通用户向自己参与的会话发送消息。第一版只支持 text body，client_message_id 用于重试幂等。
// @Tags 客户端消息
// @Accept json
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Param body body createMessageRequest true "消息"
// @Success 200 {object} successEnvelope{data=createMessageResponse}
// @Success 201 {object} successEnvelope{data=createMessageResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/{conversation_id}/messages [post]
func (s *Server) createConversationMessage(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	conversationID, err := normalizeMessageConversationID(c.Param("conversation_id"))
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}

	var req createMessageRequest
	if err := c.Bind(&req); err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", "请求格式错误")
	}

	clientMessageID, replyToMessageID, messageBody, err := normalizeCreateMessageRequest(c.Request().Context(), req)
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}

	message, created, memberUserIDs, err := s.createUserMessageWithMetadata(
		c.Request().Context(),
		user.ID,
		conversationID,
		clientMessageID,
		messageBody,
		finalizeNormalizedMessageBody,
		createMessageMetadata{
			ReplyToMessageID: replyToMessageID,
		},
	)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return failure(c, http.StatusNotFound, "not_found", "会话不存在")
		}
		if errors.Is(err, errConversationAccessDenied) {
			return failure(c, http.StatusForbidden, "forbidden", "无权访问会话")
		}
		if errors.Is(err, errConversationNotSendable) {
			return failure(c, http.StatusForbidden, "forbidden", "当前会话不能发送消息")
		}
		if errors.Is(err, errReplyToMessageInvalid) {
			return failure(c, http.StatusBadRequest, "invalid_request", "引用消息无效")
		}

		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	messageResponse, err := s.newMessageResponseForUser(c.Request().Context(), message, user.ID)
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}
	if created {
		s.sendRealtimeMessageCreatedToUsers(c.Request().Context(), memberUserIDs, message)
		if err := s.dispatchAppMessageCreatedEvent(user, message); err != nil {
			c.Logger().Warnf("dispatch app message event failed: %v", err)
		}
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}

	return success(c, status, createMessageResponse{
		Message: messageResponse,
	})
}

func normalizeMessageConversationID(rawID string) (string, error) {
	id := strings.TrimSpace(rawID)
	if id == "" {
		return "", errors.New("会话 ID 不能为空")
	}
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return "", errors.New("会话 ID 格式错误")
	}

	return parsedID.String(), nil
}

func normalizeListConversationMessagesQuery(rawBeforeSeq string, rawAfterSeq string, rawLimit string) (listConversationMessagesQuery, error) {
	beforeSeq, err := normalizeOptionalPositiveInt64(rawBeforeSeq, "before_seq")
	if err != nil {
		return listConversationMessagesQuery{}, err
	}
	afterSeq, err := normalizeOptionalPositiveInt64(rawAfterSeq, "after_seq")
	if err != nil {
		return listConversationMessagesQuery{}, err
	}
	if beforeSeq != nil && afterSeq != nil {
		return listConversationMessagesQuery{}, errors.New("before_seq 和 after_seq 不能同时传")
	}

	limit := defaultMessageHistoryLimit
	if strings.TrimSpace(rawLimit) != "" {
		parsedLimit, err := strconv.Atoi(strings.TrimSpace(rawLimit))
		if err != nil || parsedLimit <= 0 {
			return listConversationMessagesQuery{}, errors.New("limit 必须是正整数")
		}
		limit = parsedLimit
	}
	if limit > maxMessageHistoryLimit {
		limit = maxMessageHistoryLimit
	}

	return listConversationMessagesQuery{
		AfterSeq:  afterSeq,
		BeforeSeq: beforeSeq,
		Limit:     limit,
	}, nil
}

func normalizeOptionalPositiveInt64(rawValue string, fieldName string) (*int64, error) {
	value := strings.TrimSpace(rawValue)
	if value == "" {
		return nil, nil
	}

	parsedValue, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsedValue <= 0 {
		return nil, errors.New(fieldName + " 必须是正整数")
	}

	return &parsedValue, nil
}

func normalizeCreateMessageRequest(ctx context.Context, req createMessageRequest) (string, *string, json.RawMessage, error) {
	clientMessageID, err := normalizeClientMessageID(req.ClientMessageID)
	if err != nil {
		return "", nil, nil, err
	}
	replyToMessageID, err := normalizeOptionalMessageID(req.ReplyToMessageID, "引用消息 ID")
	if err != nil {
		return "", nil, nil, err
	}

	handler, err := findMessageBodyHandler(req.Body)
	if err != nil {
		return "", nil, nil, err
	}
	normalizedBody, err := handler.Normalize(ctx, req.Body)
	if err != nil {
		return "", nil, nil, err
	}

	return clientMessageID, replyToMessageID, normalizedBody, nil
}

func normalizeOptionalMessageID(rawID string, fieldName string) (*string, error) {
	id := strings.TrimSpace(rawID)
	if id == "" {
		return nil, nil
	}
	parsedID, err := uuid.Parse(id)
	if err != nil {
		return nil, errors.New(fieldName + " 格式错误")
	}
	normalizedID := parsedID.String()

	return &normalizedID, nil
}

func (s *Server) listUserConversationMessages(userID string, conversationID string, query listConversationMessagesQuery) ([]store.Message, listMessagesPageResponse, error) {
	member, err := s.requireReadableConversationMember(userID, conversationID)
	if err != nil {
		return nil, listMessagesPageResponse{}, err
	}
	visibleFromSeq := member.HistoryVisibleFromSeq
	if visibleFromSeq < 1 {
		visibleFromSeq = 1
	}

	dbQuery := s.db.
		Where("conversation_id = ? AND deleted_at IS NULL AND seq >= ?", conversationID, visibleFromSeq).
		Limit(query.Limit)

	needsReverse := false
	if query.BeforeSeq != nil {
		dbQuery = dbQuery.Where("seq < ?", *query.BeforeSeq).Order("seq DESC")
		needsReverse = true
	} else if query.AfterSeq != nil {
		dbQuery = dbQuery.Where("seq > ?", *query.AfterSeq).Order("seq ASC")
	} else {
		dbQuery = dbQuery.Order("seq DESC")
		needsReverse = true
	}

	var messages []store.Message
	if err := dbQuery.Find(&messages).Error; err != nil {
		return nil, listMessagesPageResponse{}, err
	}

	if needsReverse {
		reverseMessages(messages)
	}

	hasMoreBefore, hasMoreAfter, err := s.visibleMessagePageBounds(conversationID, visibleFromSeq, messages)
	if err != nil {
		return nil, listMessagesPageResponse{}, err
	}

	return messages, newListMessagesPageResponse(messages, query.Limit, hasMoreBefore, hasMoreAfter), nil
}

func (s *Server) requireReadableConversationMember(userID string, conversationID string) (store.ConversationMember, error) {
	var conversation store.Conversation
	if err := s.db.First(&conversation, "id = ?", conversationID).Error; err != nil {
		return store.ConversationMember{}, err
	}
	if conversation.Status != store.ConversationStatusActive {
		return store.ConversationMember{}, errConversationAccessDenied
	}

	var member store.ConversationMember
	if err := s.db.First(
		&member,
		"conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL",
		conversationID,
		store.ConversationMemberTypeUser,
		userID,
	).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return store.ConversationMember{}, errConversationAccessDenied
		}
		return store.ConversationMember{}, err
	}

	return member, nil
}

func (s *Server) visibleMessagePageBounds(conversationID string, visibleFromSeq int64, messages []store.Message) (bool, bool, error) {
	if len(messages) == 0 {
		return false, false, nil
	}

	oldestSeq := messages[0].Seq
	newestSeq := messages[len(messages)-1].Seq
	hasMoreBefore, err := s.visibleMessageExists(conversationID, visibleFromSeq, "seq < ?", oldestSeq)
	if err != nil {
		return false, false, err
	}
	hasMoreAfter, err := s.visibleMessageExists(conversationID, visibleFromSeq, "seq > ?", newestSeq)
	if err != nil {
		return false, false, err
	}

	return hasMoreBefore, hasMoreAfter, nil
}

func (s *Server) visibleMessageExists(conversationID string, visibleFromSeq int64, condition string, args ...any) (bool, error) {
	var message store.Message
	err := s.db.
		Select("id").
		Where("conversation_id = ? AND deleted_at IS NULL AND seq >= ?", conversationID, visibleFromSeq).
		Where(condition, args...).
		Limit(1).
		Take(&message).Error
	if err == nil {
		return true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}

	return false, err
}

func newListMessagesPageResponse(messages []store.Message, limit int, hasMoreBefore bool, hasMoreAfter bool) listMessagesPageResponse {
	var oldestSeq int64
	var newestSeq int64
	if len(messages) > 0 {
		oldestSeq = messages[0].Seq
		newestSeq = messages[len(messages)-1].Seq
	}

	return listMessagesPageResponse{
		HasMoreAfter:  hasMoreAfter,
		HasMoreBefore: hasMoreBefore,
		Limit:         limit,
		NewestSeq:     newestSeq,
		OldestSeq:     oldestSeq,
	}
}

func reverseMessages(messages []store.Message) {
	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}
}

func (s *Server) createUserMessage(ctx context.Context, userID string, conversationID string, clientMessageID string, body json.RawMessage, finalizeBody finalizeMessageBodyFunc) (store.Message, bool, []string, error) {
	return s.createUserMessageWithMetadata(ctx, userID, conversationID, clientMessageID, body, finalizeBody, createMessageMetadata{})
}

func (s *Server) createUserMessageWithMetadata(ctx context.Context, userID string, conversationID string, clientMessageID string, body json.RawMessage, finalizeBody finalizeMessageBodyFunc, metadata createMessageMetadata) (store.Message, bool, []string, error) {
	var created bool
	var message store.Message
	memberUserIDs := []string{}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		var conversation store.Conversation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&conversation, "id = ?", conversationID).Error; err != nil {
			return err
		}
		if conversation.Status != store.ConversationStatusActive ||
			conversation.PostingPolicy != store.ConversationPostingPolicyOpen {
			return errConversationNotSendable
		}

		var member store.ConversationMember
		if err := tx.First(
			&member,
			"conversation_id = ? AND member_type = ? AND member_id = ? AND left_at IS NULL",
			conversationID,
			store.ConversationMemberTypeUser,
			userID,
		).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errConversationAccessDenied
			}
			return err
		}

		existing, ok, err := findExistingMessageByClientMessageID(
			tx,
			conversationID,
			store.MessageSenderTypeUser,
			userID,
			clientMessageID,
		)
		if err != nil {
			return err
		}
		if ok {
			message = existing
			if err := advanceConversationMemberReadSeq(tx, conversationID, userID, existing.Seq); err != nil {
				return err
			}
			created = false
			return nil
		}

		if err := validateReplyToMessage(tx, conversationID, member.HistoryVisibleFromSeq, metadata.ReplyToMessageID); err != nil {
			return err
		}

		finalBody, summary, err := finalizeBody(ctx, body)
		if err != nil {
			return err
		}

		now := time.Now().UTC()
		message = store.Message{
			ID:               uuid.NewString(),
			ConversationID:   conversationID,
			Seq:              conversation.LastMessageSeq + 1,
			SenderType:       store.MessageSenderTypeUser,
			SenderID:         &userID,
			ClientMessageID:  &clientMessageID,
			DelegatedByType:  metadata.DelegatedByType,
			DelegatedByID:    metadata.DelegatedByID,
			DelegatedByName:  metadata.DelegatedByName,
			ReplyToMessageID: metadata.ReplyToMessageID,
			Body:             finalBody,
			Summary:          summary,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		if err := tx.Create(&message).Error; err != nil {
			return err
		}

		if err := tx.Model(&store.Conversation{}).
			Where("id = ?", conversationID).
			Updates(map[string]any{
				"last_message_at":      message.CreatedAt,
				"last_message_id":      message.ID,
				"last_message_seq":     message.Seq,
				"last_message_summary": message.Summary,
				"updated_at":           now,
			}).Error; err != nil {
			return err
		}
		if err := advanceConversationMemberReadSeq(tx, conversationID, userID, message.Seq); err != nil {
			return err
		}

		ids, err := loadActiveConversationUserIDs(tx, conversationID)
		if err != nil {
			return err
		}
		memberUserIDs = ids
		created = true
		return nil
	})
	if err != nil {
		return store.Message{}, false, nil, err
	}

	return message, created, memberUserIDs, nil
}

func advanceConversationMemberReadSeq(db *gorm.DB, conversationID string, userID string, seq int64) error {
	return db.Model(&store.ConversationMember{}).
		Where("conversation_id = ? AND member_type = ? AND member_id = ?", conversationID, store.ConversationMemberTypeUser, userID).
		Update("last_read_seq", gorm.Expr("CASE WHEN last_read_seq > ? THEN last_read_seq ELSE ? END", seq, seq)).Error
}

func findExistingMessageByClientMessageID(db *gorm.DB, conversationID string, senderType string, senderID string, clientMessageID string) (store.Message, bool, error) {
	var message store.Message
	result := db.
		Order("id ASC").
		Limit(1).
		Find(
			&message,
			"conversation_id = ? AND sender_type = ? AND sender_id = ? AND client_message_id = ?",
			conversationID,
			senderType,
			senderID,
			clientMessageID,
		)
	if result.Error != nil {
		return store.Message{}, false, result.Error
	}
	if result.RowsAffected == 0 {
		return store.Message{}, false, nil
	}

	return message, true, nil
}

func validateReplyToMessage(db *gorm.DB, conversationID string, visibleFromSeq int64, replyToMessageID *string) error {
	if replyToMessageID == nil {
		return nil
	}
	if visibleFromSeq < 1 {
		visibleFromSeq = 1
	}

	var message store.Message
	err := db.
		Select("id").
		Where("id = ? AND conversation_id = ? AND deleted_at IS NULL AND seq >= ?", *replyToMessageID, conversationID, visibleFromSeq).
		Limit(1).
		Take(&message).Error
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errReplyToMessageInvalid
	}

	return err
}

func newMessageResponse(message store.Message) messageResponse {
	senderID := ""
	if message.SenderID != nil {
		senderID = *message.SenderID
	}
	clientMessageID := ""
	if message.ClientMessageID != nil {
		clientMessageID = *message.ClientMessageID
	}

	response := messageResponse{
		ClientMessageID: clientMessageID,
		ConversationID:  message.ConversationID,
		CreatedAt:       message.CreatedAt,
		ID:              message.ID,
		Sender: messageSenderResponse{
			ID:   senderID,
			Type: message.SenderType,
		},
		Seq: message.Seq,
	}
	if message.RevokedAt == nil {
		response.Body = message.Body
	} else {
		response.RevokedAt = message.RevokedAt
		if message.RevokedByUserID != nil {
			response.RevokedByUserID = *message.RevokedByUserID
		}
	}
	if message.ReplyToMessageID != nil {
		response.ReplyToMessageID = *message.ReplyToMessageID
	}
	if message.DelegatedByType != nil && message.DelegatedByID != nil {
		response.DelegatedBy = &messageDelegatedByResponse{
			ID:   *message.DelegatedByID,
			Name: message.DelegatedByName,
			Type: *message.DelegatedByType,
		}
	}

	return response
}

func (s *Server) newMessageResponseForUser(ctx context.Context, message store.Message, userID string) (messageResponse, error) {
	response := newMessageResponse(message)
	if message.RevokedAt != nil || message.ReplyToMessageID == nil {
		return response, nil
	}

	quotedMessage, ok, err := s.findVisibleReplyToMessageForUser(ctx, message.ConversationID, *message.ReplyToMessageID, userID)
	if err != nil {
		return messageResponse{}, err
	}
	if !ok {
		return response, nil
	}

	replyTo, err := s.newMessageReplyToResponse(ctx, quotedMessage)
	if err != nil {
		return messageResponse{}, err
	}
	response.ReplyTo = &replyTo

	return response, nil
}

func (s *Server) findVisibleReplyToMessageForUser(ctx context.Context, conversationID string, replyToMessageID string, userID string) (store.Message, bool, error) {
	member, err := s.requireReadableConversationMember(userID, conversationID)
	if err != nil {
		return store.Message{}, false, err
	}
	visibleFromSeq := member.HistoryVisibleFromSeq
	if visibleFromSeq < 1 {
		visibleFromSeq = 1
	}

	var message store.Message
	err = s.db.WithContext(ctx).
		Where("id = ? AND conversation_id = ? AND deleted_at IS NULL AND seq >= ?", replyToMessageID, conversationID, visibleFromSeq).
		Limit(1).
		Take(&message).Error
	if err == nil {
		return message, true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return store.Message{}, false, nil
	}

	return store.Message{}, false, err
}

func (s *Server) newMessageReplyToResponse(ctx context.Context, message store.Message) (messageReplyToResponse, error) {
	senderID := ""
	if message.SenderID != nil {
		senderID = *message.SenderID
	}
	senderName, err := s.messageSenderName(ctx, message.SenderType, senderID)
	if err != nil {
		return messageReplyToResponse{}, err
	}

	summary := message.Summary
	if message.RevokedAt != nil {
		summary = revokedMessageSummary()
	}

	return messageReplyToResponse{
		ID: message.ID,
		Sender: messageReplyToSenderResponse{
			ID:   senderID,
			Name: senderName,
			Type: message.SenderType,
		},
		Seq:     message.Seq,
		Summary: summary,
	}, nil
}

func (s *Server) messageSenderName(ctx context.Context, senderType string, senderID string) (string, error) {
	switch senderType {
	case store.MessageSenderTypeUser:
		var user store.User
		if err := s.db.WithContext(ctx).Select("id", "name").First(&user, "id = ?", senderID).Error; err != nil {
			return "", err
		}
		return user.Name, nil
	case store.MessageSenderTypeApp:
		var app store.App
		err := s.db.WithContext(ctx).Unscoped().Select("id", "name").First(&app, "id = ?", senderID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "应用", nil
		}
		if err != nil {
			return "", err
		}
		if app.Name == "" {
			return "应用", nil
		}
		return app.Name, nil
	case store.MessageSenderTypeSystem:
		return "系统", nil
	default:
		return "", nil
	}
}

func findMessageBodyHandler(raw json.RawMessage) (messageBodyHandler, error) {
	if len(raw) == 0 {
		return nil, errors.New("消息体不能为空")
	}
	var envelope messageBodyEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, errors.New("消息体格式错误")
	}
	messageType := strings.TrimSpace(envelope.Type)
	if messageType == "" {
		return nil, errors.New("消息类型不能为空")
	}
	handler, ok := messageBodyHandlers[messageType]
	if !ok {
		return nil, errors.New("不支持的消息类型")
	}

	return handler, nil
}

func (textMessageBodyHandler) Type() string {
	return messageTypeText
}

func (handler textMessageBodyHandler) Validate(raw json.RawMessage) error {
	body, err := decodeTextMessageBody(raw)
	if err != nil {
		return err
	}
	if strings.TrimSpace(body.Type) != handler.Type() {
		return errors.New("消息类型错误")
	}
	content := strings.TrimSpace(body.Content)
	if content == "" {
		return errors.New("消息内容不能为空")
	}
	if len([]rune(content)) > maxTextMessageContentLength {
		return errors.New("消息内容不能超过 5000 个字符")
	}

	return nil
}

func (handler textMessageBodyHandler) Normalize(_ context.Context, raw json.RawMessage) (json.RawMessage, error) {
	if err := handler.Validate(raw); err != nil {
		return nil, err
	}
	body, err := decodeTextMessageBody(raw)
	if err != nil {
		return nil, err
	}
	normalizedBody, err := json.Marshal(textMessageBody{
		Type:    handler.Type(),
		Content: strings.TrimSpace(body.Content),
	})
	if err != nil {
		return nil, err
	}

	return normalizedBody, nil
}

func (textMessageBodyHandler) Summary(raw json.RawMessage) (string, error) {
	body, err := decodeTextMessageBody(raw)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(body.Content), nil
}

func decodeTextMessageBody(raw json.RawMessage) (textMessageBody, error) {
	var body textMessageBody
	if err := json.Unmarshal(raw, &body); err != nil {
		return textMessageBody{}, errors.New("消息体格式错误")
	}

	return body, nil
}

func finalizeNormalizedMessageBody(ctx context.Context, body json.RawMessage) (json.RawMessage, string, error) {
	handler, err := findMessageBodyHandler(body)
	if err != nil {
		return nil, "", err
	}
	if finalizer, ok := handler.(messageBodyFinalizer); ok {
		body, err = finalizer.Finalize(ctx, body)
		if err != nil {
			return nil, "", err
		}
	}
	summary, err := handler.Summary(body)
	if err != nil {
		return nil, "", err
	}

	return body, summary, nil
}

func staticMessageBodyFinalizer(summary string) finalizeMessageBodyFunc {
	return func(_ context.Context, body json.RawMessage) (json.RawMessage, string, error) {
		return body, summary, nil
	}
}

func (markdownMessageBodyHandler) Type() string {
	return messageTypeMarkdown
}

func (handler markdownMessageBodyHandler) Validate(raw json.RawMessage) error {
	body, err := decodeMarkdownMessageBody(raw)
	if err != nil {
		return err
	}
	if strings.TrimSpace(body.Type) != handler.Type() {
		return errors.New("消息类型错误")
	}
	content := strings.TrimSpace(body.Content)
	if content == "" {
		return errors.New("消息内容不能为空")
	}
	if len([]rune(content)) > maxTextMessageContentLength {
		return errors.New("消息内容不能超过 5000 个字符")
	}

	return nil
}

func (handler markdownMessageBodyHandler) Normalize(_ context.Context, raw json.RawMessage) (json.RawMessage, error) {
	if err := handler.Validate(raw); err != nil {
		return nil, err
	}
	body, err := decodeMarkdownMessageBody(raw)
	if err != nil {
		return nil, err
	}
	normalizedBody, err := json.Marshal(markdownMessageBody{
		Type:    handler.Type(),
		Content: strings.TrimSpace(body.Content),
	})
	if err != nil {
		return nil, err
	}

	return normalizedBody, nil
}

func (markdownMessageBodyHandler) Summary(raw json.RawMessage) (string, error) {
	body, err := decodeMarkdownMessageBody(raw)
	if err != nil {
		return "", err
	}

	return extractMarkdownPlainTextSummary(strings.TrimSpace(body.Content))
}

func decodeMarkdownMessageBody(raw json.RawMessage) (markdownMessageBody, error) {
	var body markdownMessageBody
	if err := json.Unmarshal(raw, &body); err != nil {
		return markdownMessageBody{}, errors.New("消息体格式错误")
	}

	return body, nil
}

func (linkMessageBodyHandler) Type() string {
	return messageTypeLink
}

func (handler linkMessageBodyHandler) Validate(raw json.RawMessage) error {
	body, err := decodeLinkMessageBody(raw)
	if err != nil {
		return err
	}
	if strings.TrimSpace(body.Type) != handler.Type() {
		return errors.New("消息类型错误")
	}
	if _, err := normalizeLinkMessageURL(body.URL); err != nil {
		return err
	}

	return nil
}

func (handler linkMessageBodyHandler) Normalize(_ context.Context, raw json.RawMessage) (json.RawMessage, error) {
	if err := handler.Validate(raw); err != nil {
		return nil, err
	}
	body, err := decodeLinkMessageBody(raw)
	if err != nil {
		return nil, err
	}
	normalizedURL, err := normalizeLinkMessageURL(body.URL)
	if err != nil {
		return nil, err
	}
	normalizedBody, err := json.Marshal(linkMessageBody{
		Type: handler.Type(),
		URL:  normalizedURL,
	})
	if err != nil {
		return nil, err
	}

	return normalizedBody, nil
}

func (handler linkMessageBodyHandler) Finalize(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	body, err := decodeLinkMessageBody(raw)
	if err != nil {
		return nil, err
	}
	normalizedURL, err := normalizeLinkMessageURL(body.URL)
	if err != nil {
		return nil, err
	}
	title := fetchLinkMessageTitle(ctx, normalizedURL)
	if title == "" {
		title = linkMessageFallbackTitle(normalizedURL)
	}
	finalBody, err := json.Marshal(linkMessageBody{
		Type:  handler.Type(),
		URL:   normalizedURL,
		Title: title,
	})
	if err != nil {
		return nil, err
	}

	return finalBody, nil
}

func (linkMessageBodyHandler) Summary(raw json.RawMessage) (string, error) {
	body, err := decodeLinkMessageBody(raw)
	if err != nil {
		return "", err
	}
	title := strings.TrimSpace(body.Title)
	if title == "" {
		title = linkMessageFallbackTitle(strings.TrimSpace(body.URL))
	}

	return "[链接] " + title, nil
}

func decodeLinkMessageBody(raw json.RawMessage) (linkMessageBody, error) {
	var body linkMessageBody
	if err := json.Unmarshal(raw, &body); err != nil {
		return linkMessageBody{}, errors.New("消息体格式错误")
	}

	return body, nil
}

func normalizeLinkMessageURL(rawURL string) (string, error) {
	linkURL := strings.TrimSpace(rawURL)
	if linkURL == "" {
		return "", errors.New("链接不能为空")
	}
	if len([]rune(linkURL)) > maxLinkMessageURLLength {
		return "", errors.New("链接不能超过 2048 个字符")
	}
	if strings.ContainsAny(linkURL, " \t\r\n") {
		return "", errors.New("链接格式错误")
	}
	if strings.HasPrefix(strings.ToLower(linkURL), "www.") {
		linkURL = "https://" + linkURL
	}

	parsedURL, err := url.Parse(linkURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return "", errors.New("链接格式错误")
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return "", errors.New("只支持 http 或 https 链接")
	}
	if strings.TrimSpace(parsedURL.Hostname()) == "" {
		return "", errors.New("链接格式错误")
	}

	return parsedURL.String(), nil
}

func fetchLinkMessageTitle(ctx context.Context, linkURL string) string {
	title, err := fetchLinkPreviewTitle(ctx, linkURL)
	if err != nil {
		return ""
	}

	return normalizeLinkMessageTitle(title)
}

func fetchLinkPreviewTitleHTTP(ctx context.Context, linkURL string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	requestCtx, cancel := context.WithTimeout(ctx, linkPreviewFetchTimeout)
	defer cancel()

	parsedURL, err := url.Parse(linkURL)
	if err != nil {
		return "", err
	}
	if err := validateLinkFetchURL(requestCtx, parsedURL); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, linkURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("User-Agent", "MyGod Link Preview")

	resp, err := linkPreviewHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", errors.New("link preview response status is not successful")
	}

	content, err := io.ReadAll(io.LimitReader(resp.Body, maxLinkPreviewReadBytes))
	if err != nil {
		return "", err
	}

	return extractHTMLTitle(content), nil
}

func newLinkPreviewHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: linkPreviewFetchTimeout}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network string, address string) (net.Conn, error) {
		return dialLinkPreviewAddress(ctx, dialer, network, address)
	}

	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= linkPreviewMaxRedirects {
				return errors.New("link preview redirect limit exceeded")
			}

			return validateLinkFetchURL(req.Context(), req.URL)
		},
		Timeout:   linkPreviewFetchTimeout,
		Transport: transport,
	}
}

func dialLinkPreviewAddress(ctx context.Context, dialer *net.Dialer, network string, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}

	addrs, err := resolveAllowedLinkFetchAddrs(ctx, host)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for _, addr := range addrs {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(addr.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}

	return nil, errors.New("link preview host has no address")
}

func validateLinkFetchURL(ctx context.Context, parsedURL *url.URL) error {
	if parsedURL == nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		return errors.New("link preview url is not allowed")
	}

	return validateLinkFetchHost(ctx, parsedURL.Hostname())
}

func validateLinkFetchHost(ctx context.Context, host string) error {
	_, err := resolveAllowedLinkFetchAddrs(ctx, host)

	return err
}

func resolveAllowedLinkFetchAddrs(ctx context.Context, host string) ([]netip.Addr, error) {
	normalizedHost := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if normalizedHost == "" || normalizedHost == "localhost" || strings.HasSuffix(normalizedHost, ".localhost") {
		return nil, errors.New("link preview host is not allowed")
	}

	if addr, err := netip.ParseAddr(normalizedHost); err == nil {
		if err := validateLinkFetchAddr(addr); err != nil {
			return nil, err
		}

		return []netip.Addr{addr.Unmap()}, nil
	}

	ips, err := net.DefaultResolver.LookupIPAddr(ctx, normalizedHost)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, errors.New("link preview host has no address")
	}
	addrs := make([]netip.Addr, 0, len(ips))
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip.IP)
		if !ok {
			return nil, errors.New("link preview host has invalid address")
		}
		if err := validateLinkFetchAddr(addr); err != nil {
			return nil, err
		}
		addrs = append(addrs, addr.Unmap())
	}

	return addrs, nil
}

func validateLinkFetchAddr(addr netip.Addr) error {
	addr = addr.Unmap()
	if !addr.IsValid() ||
		addr.IsUnspecified() ||
		addr.IsLoopback() ||
		addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsMulticast() ||
		linkPreviewCarrierGradeNATPrefix().Contains(addr) {
		return errors.New("link preview address is not allowed")
	}

	return nil
}

func linkPreviewCarrierGradeNATPrefix() netip.Prefix {
	return netip.MustParsePrefix("100.64.0.0/10")
}

func extractHTMLTitle(content []byte) string {
	document, err := nethtml.Parse(bytes.NewReader(content))
	if err != nil {
		return ""
	}

	return normalizeLinkMessageTitle(findHTMLTitleText(document))
}

func findHTMLTitleText(node *nethtml.Node) string {
	if node == nil {
		return ""
	}
	if node.Type == nethtml.ElementNode && strings.EqualFold(node.Data, "title") {
		var buffer strings.Builder
		collectHTMLText(node, &buffer)
		return buffer.String()
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if title := findHTMLTitleText(child); title != "" {
			return title
		}
	}

	return ""
}

func collectHTMLText(node *nethtml.Node, buffer *strings.Builder) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == nethtml.TextNode {
			buffer.WriteString(child.Data)
		}
		collectHTMLText(child, buffer)
	}
}

func normalizeLinkMessageTitle(title string) string {
	return strings.Join(strings.Fields(stdhtml.UnescapeString(title)), " ")
}

func linkMessageFallbackTitle(linkURL string) string {
	parsedURL, err := url.Parse(linkURL)
	if err != nil {
		return strings.TrimSpace(linkURL)
	}
	host := strings.TrimSpace(parsedURL.Hostname())
	if host != "" {
		return host
	}

	return strings.TrimSpace(linkURL)
}

func extractMarkdownPlainTextSummary(content string) (string, error) {
	source := []byte(content)
	document := markdownParser.Parser().Parse(text.NewReader(source))
	var buffer bytes.Buffer

	err := ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			appendMarkdownSummaryNodeText(&buffer, node, source)
			return ast.WalkContinue, nil
		}

		if node.Kind() == extensionast.KindTableCell {
			appendMarkdownSummaryCellBreak(&buffer)
		}
		if isMarkdownSummaryLineBoundary(node.Kind()) {
			appendMarkdownSummaryLineBreak(&buffer)
		}

		return ast.WalkContinue, nil
	})
	if err != nil {
		return "", err
	}

	return normalizeMarkdownSummaryText(buffer.String()), nil
}

func appendMarkdownSummaryNodeText(buffer *bytes.Buffer, node ast.Node, source []byte) {
	switch typedNode := node.(type) {
	case *ast.Text:
		buffer.Write(typedNode.Value(source))
		if typedNode.SoftLineBreak() || typedNode.HardLineBreak() {
			appendMarkdownSummaryLineBreak(buffer)
		}
	case *ast.String:
		buffer.Write(typedNode.Value)
	case *ast.CodeSpan:
		buffer.Write(typedNode.Text(source))
	case *ast.AutoLink:
		buffer.Write(typedNode.Label(source))
	case *ast.CodeBlock:
		appendMarkdownSummaryLineBreak(buffer)
		buffer.Write(typedNode.Text(source))
	case *ast.FencedCodeBlock:
		appendMarkdownSummaryLineBreak(buffer)
		buffer.Write(typedNode.Text(source))
	}
}

func isMarkdownSummaryLineBoundary(kind ast.NodeKind) bool {
	switch kind {
	case ast.KindBlockquote,
		ast.KindCodeBlock,
		ast.KindFencedCodeBlock,
		ast.KindHeading,
		ast.KindListItem,
		ast.KindParagraph,
		ast.KindThematicBreak,
		ast.KindTextBlock,
		extensionast.KindTable,
		extensionast.KindTableHeader,
		extensionast.KindTableRow:
		return true
	default:
		return false
	}
}

func appendMarkdownSummaryCellBreak(buffer *bytes.Buffer) {
	if buffer.Len() == 0 {
		return
	}
	current := buffer.String()
	if strings.HasSuffix(current, "\n") || strings.HasSuffix(current, " ") {
		return
	}
	buffer.WriteByte(' ')
}

func appendMarkdownSummaryLineBreak(buffer *bytes.Buffer) {
	if buffer.Len() == 0 {
		return
	}
	current := buffer.String()
	if strings.HasSuffix(current, "\n") {
		return
	}
	buffer.WriteByte('\n')
}

func normalizeMarkdownSummaryText(content string) string {
	rawLines := strings.Split(content, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue
		}
		lines = append(lines, trimmedLine)
	}

	return strings.Join(lines, "\n")
}

func loadActiveConversationUserIDs(db *gorm.DB, conversationID string) ([]string, error) {
	var members []store.ConversationMember
	if err := db.
		Where("conversation_id = ? AND member_type = ? AND left_at IS NULL", conversationID, store.ConversationMemberTypeUser).
		Order("joined_at ASC").
		Find(&members).Error; err != nil {
		return nil, err
	}

	userIDs := make([]string, 0, len(members))
	for _, member := range members {
		userIDs = append(userIDs, member.MemberID)
	}

	return userIDs, nil
}

func realtimeMessageCreatedEvent(message messageResponse) realtime.Envelope {
	return realtime.NewEvent(realtime.EventMessageCreated, createMessageResponse{
		Message: message,
	})
}

func realtimeMessageUpdatedEvent(message messageResponse) realtime.Envelope {
	return realtime.NewEvent(realtime.EventMessageUpdated, createMessageResponse{
		Message: message,
	})
}

type conversationRemovedEventPayload struct {
	ConversationID string `json:"conversation_id"`
}

func realtimeConversationRemovedEvent(conversationID string) realtime.Envelope {
	return realtime.NewEvent(realtime.EventConversationRemoved, conversationRemovedEventPayload{
		ConversationID: conversationID,
	})
}

func (s *Server) sendRealtimeMessageCreatedToUsers(ctx context.Context, userIDs []string, message store.Message) {
	for _, userID := range userIDs {
		response, err := s.newMessageResponseForUser(ctx, message, userID)
		if err != nil {
			response = newMessageResponse(message)
		}
		s.realtime.SendToUsers([]string{userID}, realtimeMessageCreatedEvent(response))
	}
}

func (s *Server) sendRealtimeMessageUpdatedToUsers(ctx context.Context, userIDs []string, message store.Message) {
	for _, userID := range userIDs {
		response, err := s.newMessageResponseForUser(ctx, message, userID)
		if err != nil {
			response = newMessageResponse(message)
		}
		s.realtime.SendToUsers([]string{userID}, realtimeMessageUpdatedEvent(response))
	}
}
