package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"app/internal/realtime"
	"app/internal/store"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	defaultMessageHistoryLimit  = 20
	maxMessageHistoryLimit      = 20
	maxClientMessageIDLength    = 128
	maxTextMessageContentLength = 5000
	messageTypeText             = "text"
)

var (
	errConversationAccessDenied = errors.New("conversation access denied")
	errConversationNotSendable  = errors.New("conversation not sendable")
)

type createMessageRequest struct {
	ClientMessageID string          `json:"client_message_id" example:"9c08f2dd-0af6-4e99-b486-2f0c841822be"`
	Body            json.RawMessage `json:"body" swaggertype:"object"`
}

type messageSenderResponse struct {
	ID   string `json:"id,omitempty" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	Type string `json:"type" example:"user"`
}

type messageResponse struct {
	ClientMessageID string                `json:"client_message_id" example:"9c08f2dd-0af6-4e99-b486-2f0c841822be"`
	Body            json.RawMessage       `json:"body" swaggertype:"object"`
	ConversationID  string                `json:"conversation_id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	CreatedAt       time.Time             `json:"created_at" format:"date-time"`
	ID              string                `json:"id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	Sender          messageSenderResponse `json:"sender"`
	Seq             int64                 `json:"seq" example:"13"`
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

type messageBodyEnvelope struct {
	Type string `json:"type"`
}

type messageBodyHandler interface {
	Type() string
	Validate(body json.RawMessage) error
	Normalize(body json.RawMessage) (json.RawMessage, error)
	Summary(body json.RawMessage) (string, error)
}

type textMessageBodyHandler struct{}

var messageBodyHandlers = map[string]messageBodyHandler{
	messageTypeText: textMessageBodyHandler{},
}

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
		responses = append(responses, newMessageResponse(message))
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

	clientMessageID, messageBody, summary, err := normalizeCreateMessageRequest(req)
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}

	message, created, memberUserIDs, err := s.createUserMessage(user.ID, conversationID, clientMessageID, messageBody, summary)
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

		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	messageResponse := newMessageResponse(message)
	if created {
		s.realtime.SendToUsers(memberUserIDs, realtimeMessageCreatedEvent(messageResponse))
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

func normalizeCreateMessageRequest(req createMessageRequest) (string, json.RawMessage, string, error) {
	clientMessageID := strings.TrimSpace(req.ClientMessageID)
	if clientMessageID == "" {
		return "", nil, "", errors.New("客户端消息 ID 不能为空")
	}
	if len([]rune(clientMessageID)) > maxClientMessageIDLength {
		return "", nil, "", errors.New("客户端消息 ID 不能超过 128 个字符")
	}

	handler, err := findMessageBodyHandler(req.Body)
	if err != nil {
		return "", nil, "", err
	}
	normalizedBody, err := handler.Normalize(req.Body)
	if err != nil {
		return "", nil, "", err
	}
	summary, err := handler.Summary(normalizedBody)
	if err != nil {
		return "", nil, "", err
	}

	return clientMessageID, normalizedBody, summary, nil
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

func (s *Server) createUserMessage(userID string, conversationID string, clientMessageID string, body json.RawMessage, summary string) (store.Message, bool, []string, error) {
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

		var existing store.Message
		err := tx.First(
			&existing,
			"conversation_id = ? AND sender_type = ? AND sender_id = ? AND client_message_id = ?",
			conversationID,
			store.MessageSenderTypeUser,
			userID,
			clientMessageID,
		).Error
		if err == nil {
			message = existing
			created = false
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		now := time.Now().UTC()
		message = store.Message{
			ID:              uuid.NewString(),
			ConversationID:  conversationID,
			Seq:             conversation.LastMessageSeq + 1,
			SenderType:      store.MessageSenderTypeUser,
			SenderID:        &userID,
			ClientMessageID: &clientMessageID,
			Body:            body,
			Summary:         summary,
			CreatedAt:       now,
			UpdatedAt:       now,
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

func newMessageResponse(message store.Message) messageResponse {
	senderID := ""
	if message.SenderID != nil {
		senderID = *message.SenderID
	}
	clientMessageID := ""
	if message.ClientMessageID != nil {
		clientMessageID = *message.ClientMessageID
	}

	return messageResponse{
		ClientMessageID: clientMessageID,
		Body:            message.Body,
		ConversationID:  message.ConversationID,
		CreatedAt:       message.CreatedAt,
		ID:              message.ID,
		Sender: messageSenderResponse{
			ID:   senderID,
			Type: message.SenderType,
		},
		Seq: message.Seq,
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

func (handler textMessageBodyHandler) Normalize(raw json.RawMessage) (json.RawMessage, error) {
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
