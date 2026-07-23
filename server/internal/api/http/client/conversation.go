package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"app/internal/application/account"
	conversationapp "app/internal/application/conversation"
	projectapp "app/internal/application/project"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type ConversationAPI struct {
	conversations conversationapp.ClientService
	projects      projectapp.ClientService
}

type conversationOptionalStringSlice struct {
	Present bool
	Value   []string
}

func (value *conversationOptionalStringSlice) UnmarshalJSON(raw []byte) error {
	value.Present = true
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return errors.New("字符串数组字段不能为 null")
	}
	return json.Unmarshal(raw, &value.Value)
}

type createGroupConversationRequest struct {
	AppIDs     []string                        `json:"app_ids" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	MemberIDs  []string                        `json:"member_ids" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	Name       string                          `json:"name" binding:"required" example:"产品讨论组"`
	ProjectIDs conversationOptionalStringSlice `json:"project_ids" swaggertype:"array,string" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
}

type addGroupConversationMembersRequest struct {
	AppIDs    []string `json:"app_ids" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	MemberIDs []string `json:"member_ids" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
}

type updateGroupConversationNameRequest struct {
	Name string `json:"name" example:"产品讨论组"`
}

type createAppConversationRequest struct {
	AppID string `json:"app_id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
}

type createDirectConversationRequest struct {
	UserID string `json:"user_id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
}

type markConversationReadRequest struct {
	UpToSeq *int64 `json:"up_to_seq" example:"123"`
}

type conversationMemberResponse struct {
	Avatar   string `json:"avatar" example:"/assets/avatars/builtin/07.webp"`
	Email    string `json:"email" example:"user@example.com"`
	ID       string `json:"id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	Name     string `json:"name" example:"张三"`
	Nickname string `json:"nickname" example:"小张"`
	Phone    string `json:"phone" example:"+8613812345678"`
	Role     string `json:"role" example:"member"`
	Type     string `json:"type" example:"user"`
}

type conversationProjectResponse struct {
	Avatar      string `json:"avatar"`
	Description string `json:"description"`
	ID          string `json:"id"`
	Name        string `json:"name"`
}

type conversationLastMessageSenderResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Nickname string `json:"nickname"`
	Type     string `json:"type" example:"user"`
}

type conversationListItemResponse struct {
	Avatar             string                                 `json:"avatar" example:"/assets/avatars/builtin/07.webp"`
	CanSend            bool                                   `json:"can_send" example:"true"`
	CreatedAt          time.Time                              `json:"created_at" format:"date-time"`
	ID                 string                                 `json:"id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	LastMessageAt      *time.Time                             `json:"last_message_at" format:"date-time"`
	LastMessageID      *string                                `json:"last_message_id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	LastMessageSeq     int64                                  `json:"last_message_seq" example:"12"`
	LastMessageSender  *conversationLastMessageSenderResponse `json:"last_message_sender"`
	LastMessageSummary string                                 `json:"last_message_summary" example:"好的，我看一下"`
	LastMentionedSeq   int64                                  `json:"last_mentioned_seq" example:"0"`
	LastChoiceSeq      int64                                  `json:"last_choice_seq" example:"0"`
	LastReadSeq        int64                                  `json:"last_read_seq" example:"9"`
	MemberCount        int                                    `json:"member_count" example:"2"`
	Members            []conversationMemberResponse           `json:"members"`
	Name               string                                 `json:"name" example:"张三"`
	NotificationMuted  bool                                   `json:"notification_muted" example:"false"`
	Pinned             bool                                   `json:"pinned" example:"false"`
	Projects           *[]conversationProjectResponse         `json:"projects,omitempty"`
	Type               string                                 `json:"type" example:"direct"`
	Topic              *conversationTopicMetadataResponse     `json:"topic,omitempty"`
	UnreadCount        int64                                  `json:"unread_count" example:"3"`
	Visibility         string                                 `json:"visibility" example:"private"`
}

type conversationTopicMetadataResponse struct {
	Archived               bool                      `json:"archived"`
	ParentConversationID   string                    `json:"parent_conversation_id"`
	ParentConversationName string                    `json:"parent_conversation_name"`
	ParentConversationType string                    `json:"parent_conversation_type"`
	Participating          bool                      `json:"participating"`
	SourceMessageID        string                    `json:"source_message_id"`
	SourceMessageSeq       int64                     `json:"source_message_seq"`
	SourceSender           topicSourceSenderResponse `json:"source_sender"`
}

type topicReferenceResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type topicSourceSenderResponse struct {
	Avatar string `json:"avatar"`
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
}

type topicSourceMessageResponse struct {
	Body      json.RawMessage           `json:"body,omitempty"`
	CreatedAt time.Time                 `json:"created_at"`
	ID        string                    `json:"id"`
	RevokedAt *time.Time                `json:"revoked_at,omitempty"`
	Sender    topicSourceSenderResponse `json:"sender"`
	Seq       int64                     `json:"seq"`
	Summary   string                    `json:"summary"`
}

type topicDetailResponse struct {
	CanArchive         bool                         `json:"can_archive"`
	CanParticipate     bool                         `json:"can_participate"`
	Conversation       conversationListItemResponse `json:"conversation"`
	ParentConversation topicReferenceResponse       `json:"parent_conversation"`
	SourceMessage      topicSourceMessageResponse   `json:"source_message"`
}

type createTopicResponse struct {
	Conversation conversationListItemResponse `json:"conversation"`
	Created      bool                         `json:"created"`
}

type groupConversationResponse struct {
	Avatar             string                                 `json:"avatar" example:"/assets/avatars/groups/07.webp"`
	CreatedAt          time.Time                              `json:"created_at" format:"date-time"`
	CreatedByUserID    string                                 `json:"created_by_user_id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	ID                 string                                 `json:"id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	LastMessageAt      *time.Time                             `json:"last_message_at" format:"date-time"`
	LastMessageID      *string                                `json:"last_message_id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	LastMessageSeq     int64                                  `json:"last_message_seq" example:"12"`
	LastMessageSender  *conversationLastMessageSenderResponse `json:"last_message_sender"`
	LastMessageSummary string                                 `json:"last_message_summary" example:"张三 邀请 李四 加入群聊"`
	LastMentionedSeq   int64                                  `json:"last_mentioned_seq" example:"0"`
	LastChoiceSeq      int64                                  `json:"last_choice_seq" example:"0"`
	LastReadSeq        int64                                  `json:"last_read_seq" example:"12"`
	MemberCount        int                                    `json:"member_count" example:"3"`
	Members            []conversationMemberResponse           `json:"members"`
	Name               string                                 `json:"name" example:"产品讨论组"`
	PostingPolicy      string                                 `json:"posting_policy" example:"open"`
	Status             string                                 `json:"status" example:"active"`
	Type               string                                 `json:"type" example:"group"`
	UnreadCount        int64                                  `json:"unread_count" example:"0"`
	Visibility         string                                 `json:"visibility" example:"private"`
}

type listClientConversationsResponse struct {
	Conversations []conversationListItemResponse `json:"conversations"`
}

type createDirectConversationResponse struct {
	Conversation conversationListItemResponse `json:"conversation"`
	Created      bool                         `json:"created" example:"true"`
}

type createGroupConversationResponse struct {
	Conversation groupConversationResponse `json:"conversation"`
}

type addGroupConversationMembersResponse struct {
	Conversation conversationListItemResponse `json:"conversation"`
	Message      *messageResponse             `json:"message"`
}

type updateGroupConversationAvatarResponse struct {
	Conversation conversationListItemResponse `json:"conversation"`
	Message      messageResponse              `json:"message"`
}

type leaveGroupConversationResponse struct {
	ConversationID string          `json:"conversation_id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	Message        messageResponse `json:"message"`
}

type dissolveGroupConversationResponse struct {
	ConversationID string `json:"conversation_id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
}

type markConversationReadResponse struct {
	ConversationID string `json:"conversation_id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	LastReadSeq    int64  `json:"last_read_seq" example:"123"`
	UnreadCount    int64  `json:"unread_count" example:"0"`
}

type setConversationPinResponse struct {
	ConversationID string `json:"conversation_id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	Pinned         bool   `json:"pinned" example:"true"`
}

type setConversationMuteResponse struct {
	ConversationID string `json:"conversation_id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
	Muted          bool   `json:"muted" example:"true"`
}

type dismissConversationResponse struct {
	ConversationID string `json:"conversation_id" example:"7f8d8b84-6d2c-4b12-9a8a-019a7e2787d4"`
}

type restoreConversationResponse struct {
	Conversation conversationListItemResponse `json:"conversation"`
}

func NewConversationAPI(conversations conversationapp.ClientService, projects projectapp.ClientService) *ConversationAPI {
	return &ConversationAPI{conversations: conversations, projects: projects}
}

func (a *ConversationAPI) RegisterRoutes(group *echo.Group) {
	group.GET("/conversations", a.list)
	group.POST("/conversations/apps", a.createApp)
	group.POST("/conversations/direct", a.createDirect)
	group.POST("/conversations/groups", a.createGroup)
	group.PATCH("/conversations/groups/:conversation_id/name", a.updateName)
	group.POST("/conversations/groups/:conversation_id/public", a.setPublic)
	group.POST("/conversations/groups/:conversation_id/private", a.setPrivate)
	group.POST("/conversations/groups/:conversation_id/join", a.join)
	group.POST("/conversations/groups/:conversation_id/leave", a.leave)
	group.DELETE("/conversations/groups/:conversation_id", a.dissolve)
	group.DELETE("/conversations/groups/:conversation_id/members/:member_type/:member_id", a.removeTypedMember)
	group.DELETE("/conversations/groups/:conversation_id/members/:member_id", a.removeMember)
	group.POST("/conversations/:conversation_id/avatar", a.uploadAvatar)
	group.PUT("/conversations/:conversation_id/projects/:project_id", a.bindProject)
	group.DELETE("/conversations/:conversation_id/projects/:project_id", a.unbindProject)
	group.POST("/conversations/:conversation_id/members", a.addMembers)
	group.POST("/conversations/:conversation_id/read", a.markRead)
	group.DELETE("/conversations/:conversation_id", a.dismiss)
	group.POST("/conversations/:conversation_id/restore", a.restore)
	group.PUT("/conversations/:conversation_id/mute", a.mute)
	group.DELETE("/conversations/:conversation_id/mute", a.unmute)
	group.PUT("/conversations/:conversation_id/pin", a.pin)
	group.DELETE("/conversations/:conversation_id/pin", a.unpin)
	group.POST("/conversations/:conversation_id/messages/:message_id/topic", a.createTopic)
	group.GET("/conversations/topics/:conversation_id", a.getTopic)
	group.POST("/conversations/topics/:conversation_id/participate", a.participateTopic)
	group.POST("/conversations/topics/:conversation_id/archive", a.archiveTopic)
}

// mute godoc
//
// @Summary 开启会话消息免打扰
// @Description 为当前用户开启一个有权访问会话的消息免打扰。
// @Tags 客户端会话
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Success 200 {object} successEnvelope{data=setConversationMuteResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/{conversation_id}/mute [put]
func (a *ConversationAPI) mute(c echo.Context) error {
	return a.setMuted(c, true)
}

// unmute godoc
//
// @Summary 取消会话消息免打扰
// @Description 为当前用户恢复一个有权访问会话的消息提醒。
// @Tags 客户端会话
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Success 200 {object} successEnvelope{data=setConversationMuteResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/{conversation_id}/mute [delete]
func (a *ConversationAPI) unmute(c echo.Context) error {
	return a.setMuted(c, false)
}

func (a *ConversationAPI) setMuted(c echo.Context, muted bool) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, http.StatusInternalServerError, string(conversationapp.CodeInternal), "服务端错误")
	}
	result, err := a.conversations.SetMuted(c.Request().Context(), conversationapp.SetMuteCommand{
		AccountID: current.ID, ConversationID: c.Param("conversation_id"), Muted: muted,
	})
	if err != nil {
		return writeConversationError(c, err)
	}
	return writeSuccess(c, http.StatusOK, setConversationMuteResponse{
		ConversationID: result.ConversationID, Muted: result.Muted,
	})
}

// dismiss godoc
//
// @Summary 从当前用户的对话列表移除会话
// @Description 仅从当前用户的列表暂时移除会话，不删除消息或成员关系；出现新消息后会自动恢复。
// @Tags 客户端会话
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Success 200 {object} successEnvelope{data=dismissConversationResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/{conversation_id} [delete]
func (a *ConversationAPI) dismiss(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, http.StatusInternalServerError, string(conversationapp.CodeInternal), "服务端错误")
	}
	result, err := a.conversations.Dismiss(c.Request().Context(), conversationapp.DismissCommand{
		AccountID: current.ID, ConversationID: c.Param("conversation_id"),
	})
	if err != nil {
		return writeConversationError(c, err)
	}
	return writeSuccess(c, http.StatusOK, dismissConversationResponse{ConversationID: result.ConversationID})
}

// restore godoc
//
// @Summary 将会话恢复到当前用户的对话列表
// @Description 用户主动打开一个之前从列表移除的会话时恢复它，不影响消息、成员关系和免打扰状态。
// @Tags 客户端会话
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Success 200 {object} successEnvelope{data=restoreConversationResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/{conversation_id}/restore [post]
func (a *ConversationAPI) restore(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, http.StatusInternalServerError, string(conversationapp.CodeInternal), "服务端错误")
	}
	result, err := a.conversations.Restore(c.Request().Context(), conversationapp.RestoreCommand{
		AccountID: current.ID, ConversationID: c.Param("conversation_id"),
	})
	if err != nil {
		return writeConversationError(c, err)
	}
	return writeSuccess(c, http.StatusOK, restoreConversationResponse{
		Conversation: newConversationItemResponse(result),
	})
}

// pin godoc
//
// @Summary 置顶会话
// @Description 为当前用户置顶一个有权访问的会话。置顶状态仅影响当前用户。
// @Tags 客户端会话
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Success 200 {object} successEnvelope{data=setConversationPinResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/{conversation_id}/pin [put]
func (a *ConversationAPI) pin(c echo.Context) error {
	return a.setPinned(c, true)
}

// unpin godoc
//
// @Summary 取消置顶会话
// @Description 为当前用户取消置顶。内置应用茉莉不能取消置顶。
// @Tags 客户端会话
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Success 200 {object} successEnvelope{data=setConversationPinResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 409 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/{conversation_id}/pin [delete]
func (a *ConversationAPI) unpin(c echo.Context) error {
	return a.setPinned(c, false)
}

func (a *ConversationAPI) setPinned(c echo.Context, pinned bool) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, http.StatusInternalServerError, string(conversationapp.CodeInternal), "服务端错误")
	}
	result, err := a.conversations.SetPinned(c.Request().Context(), conversationapp.SetPinCommand{
		AccountID: current.ID, ConversationID: c.Param("conversation_id"), Pinned: pinned,
	})
	if err != nil {
		return writeConversationError(c, err)
	}
	return writeSuccess(c, http.StatusOK, setConversationPinResponse{
		ConversationID: result.ConversationID, Pinned: result.Pinned,
	})
}

func (a *ConversationAPI) createTopic(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, http.StatusInternalServerError, string(conversationapp.CodeInternal), "服务端错误")
	}
	result, err := a.conversations.CreateTopic(c.Request().Context(), conversationapp.CreateTopicCommand{
		Actor: conversationActor(current), ParentConversationID: c.Param("conversation_id"), SourceMessageID: c.Param("message_id"),
	})
	if err != nil {
		return writeConversationError(c, err)
	}
	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	return writeSuccess(c, status, createTopicResponse{Conversation: newConversationItemResponse(result.Conversation), Created: result.Created})
}

func (a *ConversationAPI) getTopic(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, http.StatusInternalServerError, string(conversationapp.CodeInternal), "服务端错误")
	}
	result, err := a.conversations.GetTopic(c.Request().Context(), conversationapp.GetTopicCommand{
		Actor: conversationActor(current), TopicConversationID: c.Param("conversation_id"),
	})
	if err != nil {
		return writeConversationError(c, err)
	}
	return writeSuccess(c, http.StatusOK, newTopicDetailResponse(result))
}

func (a *ConversationAPI) participateTopic(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, http.StatusInternalServerError, string(conversationapp.CodeInternal), "服务端错误")
	}
	result, err := a.conversations.ParticipateTopic(c.Request().Context(), conversationapp.ParticipateTopicCommand{
		Actor: conversationActor(current), TopicConversationID: c.Param("conversation_id"),
	})
	if err != nil {
		return writeConversationError(c, err)
	}
	return writeSuccess(c, http.StatusOK, map[string]any{"conversation": newConversationItemResponse(result)})
}

func (a *ConversationAPI) archiveTopic(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, http.StatusInternalServerError, string(conversationapp.CodeInternal), "服务端错误")
	}
	result, err := a.conversations.ArchiveTopic(c.Request().Context(), conversationapp.ArchiveTopicCommand{
		Actor: conversationActor(current), TopicConversationID: c.Param("conversation_id"),
	})
	if err != nil {
		return writeConversationError(c, err)
	}
	return writeSuccess(c, http.StatusOK, map[string]any{"conversation": newConversationItemResponse(result)})
}

// list godoc
//
// @Summary 列出当前用户会话
// @Description 普通用户获取自己参与的最近 100 个会话。茉莉固定第一，其他置顶会话和未置顶会话分别按照最后消息时间倒序排列。
// @Tags 客户端会话
// @Produce json
// @Success 200 {object} successEnvelope{data=listClientConversationsResponse}
// @Failure 401 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations [get]
func (a *ConversationAPI) list(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, 500, string(conversationapp.CodeInternal), "服务端错误")
	}
	result, err := a.conversations.List(c.Request().Context(), conversationapp.ListCommand{AccountID: current.ID})
	if err != nil {
		return writeConversationError(c, err)
	}
	items := make([]conversationListItemResponse, 0, len(result.Conversations))
	for _, item := range result.Conversations {
		items = append(items, newConversationItemResponse(item))
	}
	return writeSuccess(c, http.StatusOK, listClientConversationsResponse{Conversations: items})
}

// createDirect godoc
//
// @Summary 创建或打开一对一会话
// @Description 普通用户创建或打开和指定用户的一对一会话。重复调用会返回已有会话，不会创建重复私聊。
// @Tags 客户端会话
// @Accept json
// @Produce json
// @Param body body createDirectConversationRequest true "一对一会话目标用户"
// @Success 200 {object} successEnvelope{data=createDirectConversationResponse}
// @Success 201 {object} successEnvelope{data=createDirectConversationResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/direct [post]
func (a *ConversationAPI) createDirect(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, 500, string(conversationapp.CodeInternal), "服务端错误")
	}
	var req createDirectConversationRequest
	if err := c.Bind(&req); err != nil {
		return writeFailure(c, 400, string(conversationapp.CodeInvalidRequest), "请求格式错误")
	}
	result, err := a.conversations.CreateDirect(c.Request().Context(), conversationapp.CreateDirectCommand{Actor: conversationActor(current), UserID: req.UserID})
	if err != nil {
		return writeConversationError(c, err)
	}
	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	return writeSuccess(c, status, createDirectConversationResponse{Conversation: newConversationItemResponse(result.Conversation), Created: result.Created})
}

// createApp godoc
//
// @Summary 创建或打开应用会话
// @Description 普通用户创建或打开和指定应用的会话。应用必须启用且对当前用户可见。
// @Tags 客户端会话
// @Accept json
// @Produce json
// @Param body body createAppConversationRequest true "应用会话目标应用"
// @Success 200 {object} successEnvelope{data=createDirectConversationResponse}
// @Success 201 {object} successEnvelope{data=createDirectConversationResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/apps [post]
func (a *ConversationAPI) createApp(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, 500, string(conversationapp.CodeInternal), "服务端错误")
	}
	var req createAppConversationRequest
	if err := c.Bind(&req); err != nil {
		return writeFailure(c, 400, string(conversationapp.CodeInvalidRequest), "请求格式错误")
	}
	result, err := a.conversations.CreateApp(c.Request().Context(), conversationapp.CreateAppCommand{Actor: conversationActor(current), AppID: req.AppID})
	if err != nil {
		return writeConversationError(c, err)
	}
	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	return writeSuccess(c, status, createDirectConversationResponse{Conversation: newConversationItemResponse(result.Conversation), Created: result.Created})
}

// createGroup godoc
//
// @Summary 创建群聊
// @Description 普通用户创建群聊。当前登录用户会自动成为群主，member_ids 和 app_ids 可选择其他成员或应用，project_ids 可选填要关联的本人普通项目。
// @Tags 客户端会话
// @Accept json
// @Produce json
// @Param body body createGroupConversationRequest true "群聊信息"
// @Success 201 {object} successEnvelope{data=createGroupConversationResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Security UserSession
// @Router /api/client/conversations/groups [post]
func (a *ConversationAPI) createGroup(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, 500, string(conversationapp.CodeInternal), "服务端错误")
	}
	var req createGroupConversationRequest
	if err := c.Bind(&req); err != nil {
		return writeFailure(c, 400, string(conversationapp.CodeInvalidRequest), "请求格式错误")
	}
	result, err := a.conversations.CreateGroup(c.Request().Context(), conversationapp.CreateGroupCommand{Actor: conversationActor(current), Name: req.Name, MemberIDs: req.MemberIDs, AppIDs: req.AppIDs, ProjectIDs: req.ProjectIDs.Value})
	if err != nil {
		return writeConversationError(c, err)
	}
	return writeSuccess(c, http.StatusCreated, createGroupConversationResponse{Conversation: newGroupResponse(result.Conversation)})
}

// addMembers godoc
//
// @Summary 添加群聊成员
// @Description 普通用户向自己参与的 active 群聊添加成员，并生成一条系统邀请消息。
// @Tags 客户端会话
// @Accept json
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Param body body addGroupConversationMembersRequest true "成员信息"
// @Success 200 {object} successEnvelope{data=addGroupConversationMembersResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/{conversation_id}/members [post]
func (a *ConversationAPI) addMembers(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, 500, string(conversationapp.CodeInternal), "服务端错误")
	}
	if err := validateConversationPath(c.Param("conversation_id")); err != nil {
		return writeFailure(c, 400, string(conversationapp.CodeInvalidRequest), err.Error())
	}
	var req addGroupConversationMembersRequest
	if err := c.Bind(&req); err != nil {
		return writeFailure(c, 400, string(conversationapp.CodeInvalidRequest), "请求格式错误")
	}
	result, err := a.conversations.AddMembers(c.Request().Context(), conversationapp.AddMembersCommand{Actor: conversationActor(current), ConversationID: c.Param("conversation_id"), MemberIDs: req.MemberIDs, AppIDs: req.AppIDs})
	if err != nil {
		return writeConversationError(c, err)
	}
	return writeSuccess(c, http.StatusOK, newMutationResponse(result))
}

// removeMember godoc
//
// @Summary 移出群聊成员
// @Description 群主或管理员将成员移出 active 群聊，并生成系统消息。群主不能被移出。
// @Tags 客户端会话
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Param member_id path string true "成员用户 ID"
// @Success 200 {object} successEnvelope{data=addGroupConversationMembersResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/groups/{conversation_id}/members/{member_id} [delete]
func (a *ConversationAPI) removeMember(c echo.Context) error {
	return a.removeMemberByType(c, conversationapp.MemberTypeUser)
}

// removeTypedMember godoc
//
// @Summary 移出群聊成员或应用
// @Description 群主或管理员将用户成员或应用成员移出 active 群聊，并生成系统消息。群主不能被移出。
// @Tags 客户端会话
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Param member_type path string true "成员类型 user|app"
// @Param member_id path string true "成员 ID"
// @Success 200 {object} successEnvelope{data=addGroupConversationMembersResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/groups/{conversation_id}/members/{member_type}/{member_id} [delete]
func (a *ConversationAPI) removeTypedMember(c echo.Context) error {
	memberType := strings.TrimSpace(c.Param("member_type"))
	if memberType != conversationapp.MemberTypeUser && memberType != conversationapp.MemberTypeApp {
		return writeFailure(c, 400, string(conversationapp.CodeInvalidRequest), "成员类型格式错误")
	}
	return a.removeMemberByType(c, memberType)
}

func (a *ConversationAPI) removeMemberByType(c echo.Context, memberType string) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, 500, string(conversationapp.CodeInternal), "服务端错误")
	}
	result, err := a.conversations.RemoveMember(c.Request().Context(), conversationapp.RemoveMemberCommand{Actor: conversationActor(current), ConversationID: c.Param("conversation_id"), MemberType: memberType, MemberID: c.Param("member_id")})
	if err != nil {
		return writeConversationError(c, err)
	}
	return writeSuccess(c, http.StatusOK, newMutationResponse(result))
}

// updateName godoc
//
// @Summary 修改群聊名称
// @Description 群主或管理员修改 active 群聊名称，并生成系统消息。
// @Tags 客户端会话
// @Accept json
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Param body body updateGroupConversationNameRequest true "群聊名称"
// @Success 200 {object} successEnvelope{data=addGroupConversationMembersResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/groups/{conversation_id}/name [patch]
func (a *ConversationAPI) updateName(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, 500, string(conversationapp.CodeInternal), "服务端错误")
	}
	if err := validateConversationPath(c.Param("conversation_id")); err != nil {
		return writeFailure(c, 400, string(conversationapp.CodeInvalidRequest), err.Error())
	}
	var req updateGroupConversationNameRequest
	if err := c.Bind(&req); err != nil {
		return writeFailure(c, 400, string(conversationapp.CodeInvalidRequest), "请求格式错误")
	}
	result, err := a.conversations.UpdateName(c.Request().Context(), conversationapp.UpdateNameCommand{Actor: conversationActor(current), ConversationID: c.Param("conversation_id"), Name: req.Name})
	if err != nil {
		return writeConversationError(c, err)
	}
	return writeSuccess(c, http.StatusOK, newMutationResponse(result))
}

// setPublic godoc
//
// @Summary 设置公开群
// @Description 群主将 active 群聊设置为公开群，并生成系统消息。
// @Tags 客户端会话
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Success 200 {object} successEnvelope{data=addGroupConversationMembersResponse}
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/groups/{conversation_id}/public [post]
func (a *ConversationAPI) setPublic(c echo.Context) error {
	return a.setVisibility(c, conversationapp.VisibilityPublic)
}

// setPrivate godoc
//
// @Summary 取消公开群
// @Description 群主将 active 群聊设置为私有群，并生成系统消息。
// @Tags 客户端会话
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Success 200 {object} successEnvelope{data=addGroupConversationMembersResponse}
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/groups/{conversation_id}/private [post]
func (a *ConversationAPI) setPrivate(c echo.Context) error {
	return a.setVisibility(c, conversationapp.VisibilityPrivate)
}

func (a *ConversationAPI) setVisibility(c echo.Context, visibility string) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, 500, string(conversationapp.CodeInternal), "服务端错误")
	}
	result, err := a.conversations.UpdateVisibility(c.Request().Context(), conversationapp.UpdateVisibilityCommand{Actor: conversationActor(current), ConversationID: c.Param("conversation_id"), Visibility: visibility})
	if err != nil {
		return writeConversationError(c, err)
	}
	return writeSuccess(c, http.StatusOK, newMutationResponse(result))
}

// join godoc
//
// @Summary 加入公开群
// @Description 普通用户加入 active 公开群，并生成系统消息。
// @Tags 客户端会话
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Success 200 {object} successEnvelope{data=addGroupConversationMembersResponse}
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/groups/{conversation_id}/join [post]
func (a *ConversationAPI) join(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, 500, string(conversationapp.CodeInternal), "服务端错误")
	}
	result, err := a.conversations.Join(c.Request().Context(), conversationapp.JoinCommand{Actor: conversationActor(current), ConversationID: c.Param("conversation_id")})
	if err != nil {
		return writeConversationError(c, err)
	}
	return writeSuccess(c, http.StatusOK, newMutationResponse(result))
}

// leave godoc
//
// @Summary 退出群聊
// @Description 当前用户退出 active 群聊，并生成系统消息。群主不能退出群聊。
// @Tags 客户端会话
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Success 200 {object} successEnvelope{data=leaveGroupConversationResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/groups/{conversation_id}/leave [post]
func (a *ConversationAPI) leave(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, 500, string(conversationapp.CodeInternal), "服务端错误")
	}
	result, err := a.conversations.Leave(c.Request().Context(), conversationapp.LeaveCommand{Actor: conversationActor(current), ConversationID: c.Param("conversation_id")})
	if err != nil {
		return writeConversationError(c, err)
	}
	return writeSuccess(c, http.StatusOK, leaveGroupConversationResponse{ConversationID: result.ConversationID, Message: newConversationMessageResponse(result.Message)})
}

// dissolve godoc
//
// @Summary 解散群聊
// @Description 群主解散 active 群聊。解散后所有成员将不再看到该群聊。
// @Tags 客户端会话
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Success 200 {object} successEnvelope{data=dissolveGroupConversationResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 409 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Security UserSession
// @Router /api/client/conversations/groups/{conversation_id} [delete]
func (a *ConversationAPI) dissolve(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, 500, string(conversationapp.CodeInternal), "服务端错误")
	}
	result, err := a.conversations.Dissolve(c.Request().Context(), conversationapp.DissolveCommand{Actor: conversationActor(current), ConversationID: c.Param("conversation_id")})
	if err != nil {
		return writeConversationError(c, err)
	}
	return writeSuccess(c, http.StatusOK, dissolveGroupConversationResponse{ConversationID: result.ConversationID})
}

// markRead godoc
//
// @Summary 标记会话已读
// @Description 普通用户把自己在指定会话中的已读位置推进到指定 seq，未指定时推进到会话当前最新消息。
// @Tags 客户端会话
// @Accept json
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Param body body markConversationReadRequest false "已读位置"
// @Success 200 {object} successEnvelope{data=markConversationReadResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/{conversation_id}/read [post]
func (a *ConversationAPI) markRead(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, 500, string(conversationapp.CodeInternal), "服务端错误")
	}
	if err := validateConversationPath(c.Param("conversation_id")); err != nil {
		return writeFailure(c, 400, string(conversationapp.CodeInvalidRequest), err.Error())
	}
	var req markConversationReadRequest
	if err := c.Bind(&req); err != nil && !errors.Is(err, io.EOF) {
		return writeFailure(c, 400, string(conversationapp.CodeInvalidRequest), "请求格式错误")
	}
	result, err := a.conversations.MarkRead(c.Request().Context(), conversationapp.ReadCommand{AccountID: current.ID, ConversationID: c.Param("conversation_id"), UpToSeq: req.UpToSeq})
	if err != nil {
		return writeConversationError(c, err)
	}
	return writeSuccess(c, http.StatusOK, markConversationReadResponse{ConversationID: result.ConversationID, LastReadSeq: result.LastReadSeq, UnreadCount: result.UnreadCount})
}

// uploadAvatar godoc
//
// @Summary 上传群聊头像
// @Description 群主或管理员上传裁切后的 WebP 群头像。头像必须是 256x256，文件会写入 public bucket，并生成一条系统消息。
// @Tags 客户端会话
// @Accept multipart/form-data
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Param file formData file true "WebP 群头像"
// @Success 200 {object} successEnvelope{data=updateGroupConversationAvatarResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 413 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/{conversation_id}/avatar [post]
func (a *ConversationAPI) uploadAvatar(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, 500, string(conversationapp.CodeInternal), "服务端错误")
	}
	authorization, err := a.conversations.AuthorizeAvatarUpdate(c.Request().Context(), conversationapp.AuthorizeAvatarCommand{
		Actor: conversationActor(current), ConversationID: c.Param("conversation_id"),
	})
	if err != nil {
		return writeConversationError(c, err)
	}
	c.Request().Body = http.MaxBytesReader(c.Response().Writer, c.Request().Body, maxAvatarRequestBytes)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		if isRequestBodyTooLarge(err) {
			return writeFailure(c, 413, string(conversationapp.CodeRequestTooLarge), "群头像文件不能超过 1MiB")
		}
		return writeFailure(c, 400, string(conversationapp.CodeInvalidRequest), "请选择要上传的群头像")
	}
	if fileHeader.Size > conversationapp.MaxAvatarUploadBytes {
		return writeFailure(c, 413, string(conversationapp.CodeRequestTooLarge), "群头像文件不能超过 1MiB")
	}
	if fileHeader.Size == 0 {
		return writeFailure(c, 400, string(conversationapp.CodeInvalidRequest), "群头像文件不能为空")
	}
	file, err := fileHeader.Open()
	if err != nil {
		return writeFailure(c, 400, string(conversationapp.CodeInvalidRequest), "读取群头像失败")
	}
	defer file.Close()
	result, err := a.conversations.UploadAvatar(c.Request().Context(), conversationapp.UploadAvatarCommand{Authorization: authorization, Size: fileHeader.Size, Content: file})
	if err != nil {
		return writeConversationError(c, err)
	}
	return writeSuccess(c, http.StatusOK, updateGroupConversationAvatarResponse{Conversation: newConversationItemResponse(result.Conversation), Message: newConversationMessageResponse(result.Message)})
}

// bindProject godoc
//
// @Summary 关联群聊项目
// @Description 群主或群管理员将当前群聊关联到一个可访问的协作项目；重复关联保持成功。
// @Tags 客户端会话
// @Produce json
// @Param conversation_id path string true "群聊 ID"
// @Param project_id path string true "项目 ID"
// @Success 200 {object} successEnvelope{data=projectGroupMutationResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 409 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/{conversation_id}/projects/{project_id} [put]
func (a *ConversationAPI) bindProject(c echo.Context) error { return a.mutateProject(c, true) }

// unbindProject godoc
//
// @Summary 解除群聊项目关联
// @Description 群主或群管理员解除当前群聊与协作项目的关联；未关联时保持成功。
// @Tags 客户端会话
// @Produce json
// @Param conversation_id path string true "群聊 ID"
// @Param project_id path string true "项目 ID"
// @Success 200 {object} successEnvelope{data=projectGroupMutationResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/{conversation_id}/projects/{project_id} [delete]
func (a *ConversationAPI) unbindProject(c echo.Context) error { return a.mutateProject(c, false) }

func (a *ConversationAPI) mutateProject(c echo.Context, bind bool) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, 500, string(conversationapp.CodeInternal), "服务端错误")
	}
	cmd := projectapp.MutateGroupCommand{AccountID: current.ID, ProjectID: c.Param("project_id"), GroupID: c.Param("conversation_id"), AllowProjectMember: true, RequireGroupManager: true}
	var err error
	if bind {
		_, err = a.projects.BindGroup(c.Request().Context(), cmd)
	} else {
		err = a.projects.UnbindGroup(c.Request().Context(), cmd)
	}
	if err != nil {
		return writeConversationProjectError(c, err)
	}
	return writeSuccess(c, http.StatusOK, map[string]any{})
}

func conversationActor(value account.Account) conversationapp.Actor {
	return conversationapp.Actor{ID: value.ID, Email: value.Email, Name: value.Name, Nickname: value.Nickname, Phone: value.Phone, Avatar: value.Avatar}
}

func newConversationItemResponse(value conversationapp.Item) conversationListItemResponse {
	result := conversationListItemResponse{Avatar: value.Avatar, CanSend: value.CanSend, CreatedAt: value.CreatedAt, ID: value.ID, LastMessageAt: value.LastMessageAt, LastMessageID: value.LastMessageID, LastMessageSeq: value.LastMessageSeq, LastMessageSummary: value.LastMessageSummary, LastMentionedSeq: value.LastMentionedSeq, LastChoiceSeq: value.LastChoiceSeq, LastReadSeq: value.LastReadSeq, MemberCount: value.MemberCount, Members: newConversationMembers(value.Members), Name: value.Name, NotificationMuted: value.NotificationMuted, Pinned: value.Pinned, Type: value.Type, UnreadCount: value.UnreadCount, Visibility: value.Visibility}
	if value.LastMessageSender != nil {
		result.LastMessageSender = &conversationLastMessageSenderResponse{
			ID: value.LastMessageSender.ID, Name: value.LastMessageSender.Name,
			Nickname: value.LastMessageSender.Nickname, Type: value.LastMessageSender.Type,
		}
	}
	if value.Projects != nil {
		projects := make([]conversationProjectResponse, 0, len(*value.Projects))
		for _, project := range *value.Projects {
			projects = append(projects, conversationProjectResponse{Avatar: project.Avatar, Description: project.Description, ID: project.ID, Name: project.Name})
		}
		result.Projects = &projects
	}
	if value.Topic != nil {
		result.Topic = &conversationTopicMetadataResponse{
			Archived: value.Topic.Archived, ParentConversationID: value.Topic.ParentConversationID,
			ParentConversationName: value.Topic.ParentConversationName, ParentConversationType: value.Topic.ParentConversationType,
			Participating: value.Topic.Participating, SourceMessageID: value.Topic.SourceMessageID, SourceMessageSeq: value.Topic.SourceMessageSeq,
			SourceSender: topicSourceSenderResponse{
				Avatar: value.Topic.SourceSender.Avatar, ID: value.Topic.SourceSender.ID,
				Name: value.Topic.SourceSender.Name, Type: value.Topic.SourceSender.Type,
			},
		}
	}
	return result
}

func newTopicDetailResponse(value conversationapp.TopicDetail) topicDetailResponse {
	return topicDetailResponse{
		CanArchive: value.CanArchive, CanParticipate: value.CanParticipate,
		Conversation:       newConversationItemResponse(value.Conversation),
		ParentConversation: topicReferenceResponse{ID: value.ParentConversation.ID, Name: value.ParentConversation.Name, Type: value.ParentConversation.Type},
		SourceMessage: topicSourceMessageResponse{
			Body: value.SourceMessage.Body, CreatedAt: value.SourceMessage.CreatedAt, ID: value.SourceMessage.ID,
			RevokedAt: value.SourceMessage.RevokedAt,
			Sender:    topicSourceSenderResponse{Avatar: value.SourceMessage.Sender.Avatar, ID: value.SourceMessage.Sender.ID, Name: value.SourceMessage.Sender.Name, Type: value.SourceMessage.Sender.Type},
			Seq:       value.SourceMessage.Seq, Summary: value.SourceMessage.Summary,
		},
	}
}

func newGroupResponse(value conversationapp.Group) groupConversationResponse {
	result := groupConversationResponse{Avatar: value.Avatar, CreatedAt: value.CreatedAt, CreatedByUserID: value.CreatedByUserID, ID: value.ID, LastMessageAt: value.LastMessageAt, LastMessageID: value.LastMessageID, LastMessageSeq: value.LastMessageSeq, LastMessageSummary: value.LastMessageSummary, LastMentionedSeq: value.LastMentionedSeq, LastChoiceSeq: value.LastChoiceSeq, LastReadSeq: value.LastReadSeq, MemberCount: value.MemberCount, Members: newConversationMembers(value.Members), Name: value.Name, PostingPolicy: value.PostingPolicy, Status: value.Status, Type: value.Type, UnreadCount: value.UnreadCount, Visibility: value.Visibility}
	if value.LastMessageSender != nil {
		result.LastMessageSender = &conversationLastMessageSenderResponse{
			ID: value.LastMessageSender.ID, Name: value.LastMessageSender.Name,
			Nickname: value.LastMessageSender.Nickname, Type: value.LastMessageSender.Type,
		}
	}
	return result
}

func newConversationMembers(values []conversationapp.Member) []conversationMemberResponse {
	result := make([]conversationMemberResponse, 0, len(values))
	for _, value := range values {
		result = append(result, conversationMemberResponse{Avatar: value.Avatar, Email: value.Email, ID: value.ID, Name: value.Name, Nickname: value.Nickname, Phone: value.Phone, Role: value.Role, Type: value.Type})
	}
	return result
}

func newMutationResponse(value conversationapp.ConversationMutationResult) addGroupConversationMembersResponse {
	var message *messageResponse
	if value.Message != nil {
		converted := newConversationMessageResponse(*value.Message)
		message = &converted
	}
	return addGroupConversationMembersResponse{Conversation: newConversationItemResponse(value.Conversation), Message: message}
}

func newConversationMessageResponse(message conversationapp.Message) messageResponse {
	response := messageResponse{ClientMessageID: message.ClientMessageID, ConversationID: message.ConversationID, CreatedAt: message.CreatedAt, ID: message.ID, Reactions: []messageReactionResponse{}, Sender: messageSenderResponse{ID: message.Sender.ID, Type: message.Sender.Type}, Seq: message.Seq}
	if message.RevokedAt == nil {
		response.Body = message.Body
	} else {
		response.RevokedAt = message.RevokedAt
		response.RevokedByUserID = message.RevokedByUserID
	}
	response.ReplyToMessageID = message.ReplyToMessageID
	if message.DelegatedBy != nil {
		response.DelegatedBy = &messageDelegatedByResponse{ID: message.DelegatedBy.ID, Name: message.DelegatedBy.Name, Type: message.DelegatedBy.Type}
	}
	return response
}

func writeConversationError(c echo.Context, err error) error {
	status := http.StatusInternalServerError
	switch conversationapp.ErrorCodeOf(err) {
	case conversationapp.CodeInvalidRequest:
		status = http.StatusBadRequest
	case conversationapp.CodeForbidden:
		status = http.StatusForbidden
	case conversationapp.CodeNotFound:
		status = http.StatusNotFound
	case conversationapp.CodeConflict:
		status = http.StatusConflict
	case conversationapp.CodeRequestTooLarge:
		status = http.StatusRequestEntityTooLarge
	}
	return writeFailure(c, status, string(conversationapp.ErrorCodeOf(err)), conversationapp.ErrorMessage(err))
}

func writeConversationProjectError(c echo.Context, err error) error {
	message := projectapp.ErrorMessage(err)
	switch projectapp.ErrorCodeOf(err) {
	case projectapp.CodeInvalidRequest:
		if message == "只能关联群聊" {
			message = "只能管理群聊的关联项目"
		}
		return writeFailure(c, 400, "invalid_request", message)
	case projectapp.CodeForbidden:
		switch message {
		case "无权访问会话":
			message = "无权访问群聊"
		case "只有群主或群管理员可以管理群聊项目":
			message = "只有群主或管理员可以管理关联项目"
		}
		return writeFailure(c, 403, "forbidden", message)
	case projectapp.CodeNotFound:
		return writeFailure(c, 404, "not_found", "群聊或项目不存在")
	case projectapp.CodeConflict:
		return writeFailure(c, 409, "conflict", message)
	default:
		return writeFailure(c, 500, "internal_error", "服务端错误")
	}
}

func validateConversationPath(raw string) error {
	id := strings.TrimSpace(raw)
	if id == "" {
		return errors.New("会话 ID 不能为空")
	}
	if _, err := uuid.Parse(id); err != nil {
		return errors.New("会话 ID 格式错误")
	}
	return nil
}
