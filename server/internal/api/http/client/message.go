package client

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	fileapp "app/internal/application/file"
	messageapp "app/internal/application/message"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type MessageAPI struct {
	messages messageapp.ClientService
	files    fileapp.TemporaryFileService
}

type createMessageRequest struct {
	ClientMessageID  string          `json:"client_message_id" example:"9c08f2dd-0af6-4e99-b486-2f0c841822be"`
	ReplyToMessageID string          `json:"reply_to_message_id,omitempty" example:"9c08f2dd-0af6-4e99-b486-2f0c841822be"`
	Body             json.RawMessage `json:"body" swaggertype:"object"`
}

type setMessageReactionRequest struct {
	Reacted bool   `json:"reacted"`
	Text    string `json:"text"`
}

type listMessageReactionSnapshotsRequest struct {
	MessageIDs []string `json:"message_ids"`
}

type submitChoiceResponseRequest struct {
	OptionIDs []string `json:"option_ids"`
}

type listChoiceSnapshotsRequest struct {
	MessageIDs []string `json:"message_ids"`
}

type messageChoiceOptionStateResponse struct {
	ID            string `json:"id"`
	ResponseCount int64  `json:"response_count"`
}

type messageChoiceStateResponse struct {
	MyOptionIDs   []string                           `json:"my_option_ids"`
	Options       []messageChoiceOptionStateResponse `json:"options"`
	ResponseCount int64                              `json:"response_count"`
}

type choiceResponseResponse struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
	OptionIDs []string  `json:"option_ids"`
	UserID    string    `json:"user_id"`
}

type submitChoiceResponseResponse struct {
	Choice         messageChoiceStateResponse `json:"choice"`
	ConversationID string                     `json:"conversation_id"`
	Created        bool                       `json:"created"`
	MessageID      string                     `json:"message_id"`
	Response       choiceResponseResponse     `json:"response"`
}

type messageChoiceSnapshotResponse struct {
	Choice    *messageChoiceStateResponse `json:"choice,omitempty"`
	MessageID string                      `json:"message_id"`
	Status    string                      `json:"status"`
}

type listChoiceSnapshotsResponse struct {
	ConversationID string                          `json:"conversation_id"`
	Snapshots      []messageChoiceSnapshotResponse `json:"snapshots"`
}

type messageReactionUserResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type messageReactionResponse struct {
	Count       int64                         `json:"count"`
	ReactedByMe bool                          `json:"reacted_by_me"`
	Text        string                        `json:"text"`
	Users       []messageReactionUserResponse `json:"users"`
}

type setMessageReactionResponse struct {
	ConversationID  string                    `json:"conversation_id"`
	MessageID       string                    `json:"message_id"`
	ReactionVersion int64                     `json:"reaction_version"`
	Reactions       []messageReactionResponse `json:"reactions"`
}

type messageReactionSnapshotResponse struct {
	MessageID       string                    `json:"message_id"`
	ReactionVersion int64                     `json:"reaction_version"`
	Reactions       []messageReactionResponse `json:"reactions"`
}

type listMessageReactionSnapshotsResponse struct {
	ConversationID string                            `json:"conversation_id"`
	Snapshots      []messageReactionSnapshotResponse `json:"snapshots"`
}

type listMessageReactionUsersResponse struct {
	ConversationID string                        `json:"conversation_id"`
	MessageID      string                        `json:"message_id"`
	Text           string                        `json:"text"`
	Users          []messageReactionUserResponse `json:"users"`
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
	ReactionVersion  int64                       `json:"reaction_version"`
	Reactions        []messageReactionResponse   `json:"reactions"`
	Choice           *messageChoiceStateResponse `json:"choice,omitempty"`
	RevokedAt        *time.Time                  `json:"revoked_at,omitempty" format:"date-time"`
	RevokedByUserID  string                      `json:"revoked_by_user_id,omitempty" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	Sender           messageSenderResponse       `json:"sender"`
	Seq              int64                       `json:"seq" example:"13"`
	Topic            *messageTopicResponse       `json:"topic,omitempty"`
}

type messageTopicResponse struct {
	Archived       bool                        `json:"archived"`
	ConversationID string                      `json:"conversation_id"`
	RecentReplies  []messageTopicReplyResponse `json:"recent_replies"`
}

type messageTopicReplyResponse struct {
	CreatedAt time.Time             `json:"created_at"`
	ID        string                `json:"id"`
	Sender    messageSenderResponse `json:"sender"`
	Summary   string                `json:"summary"`
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

func NewMessageAPI(messages messageapp.ClientService, files fileapp.TemporaryFileService) *MessageAPI {
	return &MessageAPI{messages: messages, files: files}
}

func (a *MessageAPI) RegisterRoutes(group *echo.Group) {
	group.GET("/conversations/:conversation_id/messages", a.list)
	group.POST("/conversations/:conversation_id/messages", a.create)
	group.POST("/conversations/:conversation_id/messages/files", a.createFile)
	group.POST("/conversations/:conversation_id/messages/images", a.createImage)
	group.POST("/conversations/:conversation_id/messages/voices", a.createVoice)
	group.POST("/conversations/:conversation_id/messages/forward", a.forward)
	group.POST("/conversations/:conversation_id/messages/reactions/query", a.listReactionSnapshots)
	group.POST("/conversations/:conversation_id/messages/choices/query", a.listChoiceSnapshots)
	group.POST("/conversations/:conversation_id/messages/:message_id/revoke", a.revoke)
	group.GET("/conversations/:conversation_id/messages/:message_id/reactions/users", a.listReactionUsers)
	group.PUT("/conversations/:conversation_id/messages/:message_id/reactions", a.setReaction)
	group.PUT("/conversations/:conversation_id/messages/:message_id/choice-response", a.submitChoiceResponse)
}

// list godoc
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
func (a *MessageAPI) list(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, http.StatusInternalServerError, string(messageapp.CodeInternal), "服务端错误")
	}
	conversationID, err := normalizeMessageConversationID(c.Param("conversation_id"))
	if err != nil {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), err.Error())
	}
	afterSeq, err := normalizeOptionalPositiveInt64(c.QueryParam("after_seq"), "after_seq")
	if err != nil {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), err.Error())
	}
	beforeSeq, err := normalizeOptionalPositiveInt64(c.QueryParam("before_seq"), "before_seq")
	if err != nil {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), err.Error())
	}
	if afterSeq != nil && beforeSeq != nil {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), "before_seq 和 after_seq 不能同时传")
	}
	limit, err := normalizeMessageHistoryLimit(c.QueryParam("limit"))
	if err != nil {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), err.Error())
	}
	result, err := a.messages.List(c.Request().Context(), messageapp.ListCommand{
		AccountID: current.ID, ConversationID: conversationID,
		AfterSeq: afterSeq, BeforeSeq: beforeSeq, Limit: limit,
	})
	if err != nil {
		return writeMessageError(c, err)
	}
	messages := make([]messageResponse, 0, len(result.Messages))
	for _, value := range result.Messages {
		messages = append(messages, newClientMessageResponse(value))
	}
	return writeSuccess(c, http.StatusOK, listConversationMessagesResponse{
		Messages: messages,
		Page: listMessagesPageResponse{
			HasMoreAfter: result.Page.HasMoreAfter, HasMoreBefore: result.Page.HasMoreBefore,
			Limit: result.Page.Limit, NewestSeq: result.Page.NewestSeq, OldestSeq: result.Page.OldestSeq,
		},
	})
}

// create godoc
//
// @Summary 发送消息
// @Description 普通用户向自己参与的会话发送 text、markdown、link、card、chart，或通过 entity_card 对象引用生成卡片消息，client_message_id 用于重试幂等。
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
// @Failure 413 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/{conversation_id}/messages [post]
func (a *MessageAPI) create(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, http.StatusInternalServerError, string(messageapp.CodeInternal), "服务端错误")
	}
	conversationID, err := normalizeMessageConversationID(c.Param("conversation_id"))
	if err != nil {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), err.Error())
	}
	c.Request().Body = http.MaxBytesReader(c.Response().Writer, c.Request().Body, messageapp.MaxCreateRequestBody)
	var request createMessageRequest
	if err := c.Bind(&request); err != nil {
		if isRequestBodyTooLarge(err) {
			return writeFailure(c, http.StatusRequestEntityTooLarge, string(messageapp.CodeRequestTooLarge), "消息内容不能超过 64 KiB")
		}
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), "请求格式错误")
	}
	result, err := a.messages.Create(c.Request().Context(), messageapp.CreateCommand{
		AccountID: current.ID, Body: request.Body, ClientMessageID: request.ClientMessageID,
		ConversationID: conversationID, ReplyToMessageID: request.ReplyToMessageID,
	})
	if err != nil {
		return writeMessageError(c, err)
	}
	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	return writeSuccess(c, status, createMessageResponse{Message: newClientMessageResponse(result.Message)})
}

func newClientMessageResponse(value messageapp.Message) messageResponse {
	reactions := newMessageReactionResponses(value.Reactions)
	result := messageResponse{
		ClientMessageID: value.ClientMessageID, Body: value.Body, ConversationID: value.ConversationID,
		CreatedAt: value.CreatedAt, ID: value.ID, ReplyToMessageID: value.ReplyToMessageID,
		ReactionVersion: value.ReactionVersion, Reactions: reactions,
		RevokedAt: value.RevokedAt, RevokedByUserID: value.RevokedByUserID,
		Sender: messageSenderResponse{ID: value.Sender.ID, Type: value.Sender.Type}, Seq: value.Seq,
	}
	if value.Choice != nil {
		choice := newMessageChoiceStateResponse(*value.Choice)
		result.Choice = &choice
	}
	if value.DelegatedBy != nil {
		result.DelegatedBy = &messageDelegatedByResponse{ID: value.DelegatedBy.ID, Name: value.DelegatedBy.Name, Type: value.DelegatedBy.Type}
	}
	if value.ReplyTo != nil {
		result.ReplyTo = &messageReplyToResponse{
			ID: value.ReplyTo.ID,
			Sender: messageReplyToSenderResponse{
				ID: value.ReplyTo.Sender.ID, Name: value.ReplyTo.Sender.Name, Type: value.ReplyTo.Sender.Type,
			},
			Seq: value.ReplyTo.Seq, Summary: value.ReplyTo.Summary,
		}
	}
	if value.Topic != nil {
		recentReplies := make([]messageTopicReplyResponse, len(value.Topic.RecentReplies))
		for index, reply := range value.Topic.RecentReplies {
			recentReplies[index] = messageTopicReplyResponse{
				CreatedAt: reply.CreatedAt, ID: reply.ID,
				Sender: messageSenderResponse{ID: reply.Sender.ID, Type: reply.Sender.Type}, Summary: reply.Summary,
			}
		}
		result.Topic = &messageTopicResponse{
			Archived: value.Topic.Archived, ConversationID: value.Topic.ConversationID,
			RecentReplies: recentReplies,
		}
	}
	return result
}

func newMessageChoiceStateResponse(value messageapp.ChoiceState) messageChoiceStateResponse {
	options := make([]messageChoiceOptionStateResponse, len(value.Options))
	for index, option := range value.Options {
		options[index] = messageChoiceOptionStateResponse{ID: option.ID, ResponseCount: option.ResponseCount}
	}
	myOptionIDs := make([]string, len(value.MyOptionIDs))
	copy(myOptionIDs, value.MyOptionIDs)
	return messageChoiceStateResponse{
		MyOptionIDs: myOptionIDs, Options: options, ResponseCount: value.ResponseCount,
	}
}

func newMessageReactionResponses(values []messageapp.ReactionSummary) []messageReactionResponse {
	result := make([]messageReactionResponse, len(values))
	for index, value := range values {
		result[index] = messageReactionResponse{
			Count: value.Count, ReactedByMe: value.ReactedByMe, Text: value.Text,
			Users: newMessageReactionUserResponses(value.Users),
		}
	}
	return result
}

func newMessageReactionUserResponses(values []messageapp.ReactionUser) []messageReactionUserResponse {
	result := make([]messageReactionUserResponse, len(values))
	for index, value := range values {
		result[index] = messageReactionUserResponse{ID: value.ID, Name: value.Name}
	}
	return result
}

// setReaction godoc
//
// @Summary 添加或移除消息表情
// @Description 普通用户为可见消息添加或移除一条文本表情。
// @Tags 客户端消息
// @Accept json
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Param message_id path string true "消息 ID"
// @Param body body setMessageReactionRequest true "表情状态"
// @Success 200 {object} successEnvelope{data=setMessageReactionResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 409 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/{conversation_id}/messages/{message_id}/reactions [put]
func (a *MessageAPI) setReaction(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, http.StatusInternalServerError, string(messageapp.CodeInternal), "服务端错误")
	}
	conversationID, err := normalizeMessageConversationID(c.Param("conversation_id"))
	if err != nil {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), err.Error())
	}
	messageID, err := normalizeMessageID(c.Param("message_id"))
	if err != nil {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), err.Error())
	}
	c.Request().Body = http.MaxBytesReader(c.Response().Writer, c.Request().Body, messageapp.MaxSetReactionRequestBody)
	var request setMessageReactionRequest
	if err := c.Bind(&request); err != nil {
		if isRequestBodyTooLarge(err) {
			return writeFailure(c, http.StatusRequestEntityTooLarge, string(messageapp.CodeRequestTooLarge), "请求内容过大")
		}
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), "请求格式错误")
	}
	result, err := a.messages.SetReaction(c.Request().Context(), messageapp.SetReactionCommand{
		AccountID: current.ID, ConversationID: conversationID, MessageID: messageID,
		Reacted: request.Reacted, Text: request.Text,
	})
	if err != nil {
		return writeMessageError(c, err)
	}
	return writeSuccess(c, http.StatusOK, setMessageReactionResponse{
		ConversationID: result.ConversationID, MessageID: result.MessageID,
		ReactionVersion: result.ReactionVersion, Reactions: newMessageReactionResponses(result.Reactions),
	})
}

// submitChoiceResponse godoc
//
// @Summary 回复选择消息
// @Description 当前用户为一条可见且未撤回的 choice 消息提交一次单选或多选答案；重复提交相同答案按幂等请求处理。
// @Tags 客户端消息
// @Accept json
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Param message_id path string true "选择消息 ID"
// @Param body body submitChoiceResponseRequest true "所选选项"
// @Success 200 {object} successEnvelope{data=submitChoiceResponseResponse}
// @Success 201 {object} successEnvelope{data=submitChoiceResponseResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 409 {object} errorEnvelope
// @Router /api/client/conversations/{conversation_id}/messages/{message_id}/choice-response [put]
func (a *MessageAPI) submitChoiceResponse(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, http.StatusInternalServerError, string(messageapp.CodeInternal), "服务端错误")
	}
	conversationID, err := normalizeMessageConversationID(c.Param("conversation_id"))
	if err != nil {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), err.Error())
	}
	messageID, err := normalizeMessageID(c.Param("message_id"))
	if err != nil {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), err.Error())
	}
	c.Request().Body = http.MaxBytesReader(c.Response().Writer, c.Request().Body, messageapp.MaxChoiceResponseBody)
	var request submitChoiceResponseRequest
	if err := c.Bind(&request); err != nil {
		if isRequestBodyTooLarge(err) {
			return writeFailure(c, http.StatusRequestEntityTooLarge, string(messageapp.CodeRequestTooLarge), "请求内容过大")
		}
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), "请求格式错误")
	}
	result, err := a.messages.SubmitChoiceResponse(c.Request().Context(), messageapp.SubmitChoiceResponseCommand{
		AccountID: current.ID, ConversationID: conversationID, MessageID: messageID, OptionIDs: request.OptionIDs,
	})
	if err != nil {
		return writeMessageError(c, err)
	}
	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	return writeSuccess(c, status, submitChoiceResponseResponse{
		Choice: newMessageChoiceStateResponse(result.Choice), ConversationID: result.ConversationID,
		Created: result.Created, MessageID: result.MessageID,
		Response: choiceResponseResponse{
			CreatedAt: result.Response.CreatedAt, ID: result.Response.ID,
			OptionIDs: append([]string(nil), result.Response.OptionIDs...), UserID: result.Response.UserID,
		},
	})
}

// listChoiceSnapshots godoc
//
// @Summary 批量查询选择消息状态
// @Description 返回当前用户视角下至多 100 条 choice 消息的聚合结果和本人答案，用于客户端断线恢复。
// @Tags 客户端消息
// @Accept json
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Param body body listChoiceSnapshotsRequest true "选择消息 ID 列表"
// @Success 200 {object} successEnvelope{data=listChoiceSnapshotsResponse}
// @Router /api/client/conversations/{conversation_id}/messages/choices/query [post]
func (a *MessageAPI) listChoiceSnapshots(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, http.StatusInternalServerError, string(messageapp.CodeInternal), "服务端错误")
	}
	conversationID, err := normalizeMessageConversationID(c.Param("conversation_id"))
	if err != nil {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), err.Error())
	}
	c.Request().Body = http.MaxBytesReader(c.Response().Writer, c.Request().Body, messageapp.MaxChoiceSnapshotBody)
	var request listChoiceSnapshotsRequest
	if err := c.Bind(&request); err != nil {
		if isRequestBodyTooLarge(err) {
			return writeFailure(c, http.StatusRequestEntityTooLarge, string(messageapp.CodeRequestTooLarge), "请求内容过大")
		}
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), "请求格式错误")
	}
	result, err := a.messages.ListChoiceSnapshots(c.Request().Context(), messageapp.ListChoiceSnapshotsCommand{
		AccountID: current.ID, ConversationID: conversationID, MessageIDs: request.MessageIDs,
	})
	if err != nil {
		return writeMessageError(c, err)
	}
	snapshots := make([]messageChoiceSnapshotResponse, len(result.Snapshots))
	for index, snapshot := range result.Snapshots {
		snapshots[index] = messageChoiceSnapshotResponse{
			MessageID: snapshot.MessageID, Status: snapshot.Status,
		}
		if snapshot.Choice != nil {
			choice := newMessageChoiceStateResponse(*snapshot.Choice)
			snapshots[index].Choice = &choice
		}
	}
	return writeSuccess(c, http.StatusOK, listChoiceSnapshotsResponse{
		ConversationID: result.ConversationID, Snapshots: snapshots,
	})
}

// listReactionSnapshots godoc
//
// @Summary 批量查询消息表情快照
// @Description 返回当前用户视角下至多 100 条可见消息的完整表情状态，用于客户端断线恢复。
// @Tags 客户端消息
// @Accept json
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Param body body listMessageReactionSnapshotsRequest true "消息 ID 列表"
// @Success 200 {object} successEnvelope{data=listMessageReactionSnapshotsResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 413 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/{conversation_id}/messages/reactions/query [post]
func (a *MessageAPI) listReactionSnapshots(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, http.StatusInternalServerError, string(messageapp.CodeInternal), "服务端错误")
	}
	conversationID, err := normalizeMessageConversationID(c.Param("conversation_id"))
	if err != nil {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), err.Error())
	}
	c.Request().Body = http.MaxBytesReader(c.Response().Writer, c.Request().Body, messageapp.MaxReactionSnapshotBody)
	var request listMessageReactionSnapshotsRequest
	if err := c.Bind(&request); err != nil {
		if isRequestBodyTooLarge(err) {
			return writeFailure(c, http.StatusRequestEntityTooLarge, string(messageapp.CodeRequestTooLarge), "请求内容过大")
		}
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), "请求格式错误")
	}
	result, err := a.messages.ListReactionSnapshots(c.Request().Context(), messageapp.ListReactionSnapshotsCommand{
		AccountID: current.ID, ConversationID: conversationID, MessageIDs: request.MessageIDs,
	})
	if err != nil {
		return writeMessageError(c, err)
	}
	snapshots := make([]messageReactionSnapshotResponse, len(result.Snapshots))
	for index, snapshot := range result.Snapshots {
		snapshots[index] = messageReactionSnapshotResponse{
			MessageID: snapshot.MessageID, ReactionVersion: snapshot.ReactionVersion,
			Reactions: newMessageReactionResponses(snapshot.Reactions),
		}
	}
	return writeSuccess(c, http.StatusOK, listMessageReactionSnapshotsResponse{
		ConversationID: result.ConversationID, Snapshots: snapshots,
	})
}

// listReactionUsers godoc
//
// @Summary 查询消息表情参与者
// @Description 返回当前用户可见消息中指定表情的完整参与者列表。
// @Tags 客户端消息
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Param message_id path string true "消息 ID"
// @Param text query string true "表情内容"
// @Success 200 {object} successEnvelope{data=listMessageReactionUsersResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/{conversation_id}/messages/{message_id}/reactions/users [get]
func (a *MessageAPI) listReactionUsers(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, http.StatusInternalServerError, string(messageapp.CodeInternal), "服务端错误")
	}
	conversationID, err := normalizeMessageConversationID(c.Param("conversation_id"))
	if err != nil {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), err.Error())
	}
	messageID, err := normalizeMessageID(c.Param("message_id"))
	if err != nil {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), err.Error())
	}
	result, err := a.messages.ListReactionUsers(c.Request().Context(), messageapp.ListReactionUsersCommand{
		AccountID: current.ID, ConversationID: conversationID, MessageID: messageID,
		Text: c.QueryParam("text"),
	})
	if err != nil {
		return writeMessageError(c, err)
	}
	return writeSuccess(c, http.StatusOK, listMessageReactionUsersResponse{
		ConversationID: result.ConversationID, MessageID: result.MessageID, Text: result.Text,
		Users: newMessageReactionUserResponses(result.Users),
	})
}

func normalizeMessageID(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errors.New("消息 ID 不能为空")
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return "", errors.New("消息 ID 格式错误")
	}
	return parsed.String(), nil
}

func normalizeMessageConversationID(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errors.New("会话 ID 不能为空")
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return "", errors.New("会话 ID 格式错误")
	}
	return parsed.String(), nil
}

func normalizeOptionalPositiveInt64(raw, field string) (*int64, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return nil, errors.New(field + " 必须是正整数")
	}
	return &parsed, nil
}

func normalizeMessageHistoryLimit(raw string) (int, error) {
	limit := messageapp.DefaultHistoryLimit
	if strings.TrimSpace(raw) != "" {
		parsed, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || parsed <= 0 {
			return 0, errors.New("limit 必须是正整数")
		}
		limit = parsed
	}
	if limit > messageapp.MaxHistoryLimit {
		limit = messageapp.MaxHistoryLimit
	}
	return limit, nil
}

func writeMessageError(c echo.Context, err error) error {
	status := http.StatusInternalServerError
	switch messageapp.ErrorCodeOf(err) {
	case messageapp.CodeInvalidRequest:
		status = http.StatusBadRequest
	case messageapp.CodeForbidden:
		status = http.StatusForbidden
	case messageapp.CodeNotFound:
		status = http.StatusNotFound
	case messageapp.CodeConflict:
		status = http.StatusConflict
	case messageapp.CodeSourceUnavailable, messageapp.CodeContentUnavailable:
		status = http.StatusConflict
	case messageapp.CodeUnsupportedMessage:
		status = http.StatusBadRequest
	case messageapp.CodeRequestTooLarge:
		status = http.StatusRequestEntityTooLarge
	}
	return writeFailure(c, status, string(messageapp.ErrorCodeOf(err)), messageapp.ErrorMessage(err))
}
