package httpserver

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"app/internal/store"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// uploadGroupConversationAvatar godoc
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
func (s *Server) uploadGroupConversationAvatar(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	conversationID, err := normalizeMessageConversationID(c.Param("conversation_id"))
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}

	if err := s.requireGroupConversationAvatarUpdatePermission(user.ID, conversationID); err != nil {
		return groupConversationAvatarFailure(c, err)
	}

	c.Request().Body = http.MaxBytesReader(c.Response().Writer, c.Request().Body, maxCurrentUserAvatarRequestBytes)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		if isRequestBodyTooLarge(err) {
			return failure(c, http.StatusRequestEntityTooLarge, "request_too_large", "群头像文件不能超过 1MiB")
		}
		return failure(c, http.StatusBadRequest, "invalid_request", "请选择要上传的群头像")
	}
	if fileHeader.Size > maxCurrentUserAvatarUploadBytes {
		return failure(c, http.StatusRequestEntityTooLarge, "request_too_large", "群头像文件不能超过 1MiB")
	}
	if fileHeader.Size == 0 {
		return failure(c, http.StatusBadRequest, "invalid_request", "群头像文件不能为空")
	}

	file, err := fileHeader.Open()
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", "读取群头像失败")
	}
	defer file.Close()

	avatarBytes, err := readCurrentUserAvatarUpload(file)
	if err != nil {
		if errors.Is(err, errCurrentUserAvatarTooLarge) {
			return failure(c, http.StatusRequestEntityTooLarge, "request_too_large", "群头像文件不能超过 1MiB")
		}
		return failure(c, http.StatusBadRequest, "invalid_request", "读取群头像失败")
	}

	width, height, err := parseWebPDimensions(avatarBytes)
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", "群头像必须是 WebP 图片")
	}
	if width != currentUserAvatarOutputSize || height != currentUserAvatarOutputSize {
		return failure(c, http.StatusBadRequest, "invalid_request", "群头像尺寸必须是 256x256")
	}

	storageClient, err := s.newObjectStoreClient(c.Request().Context())
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "群头像存储未配置")
	}

	objectKey := buildGroupConversationAvatarObjectKey(conversationID, uuid.NewString())
	if err := storageClient.PutPublicObject(
		c.Request().Context(),
		objectKey,
		bytes.NewReader(avatarBytes),
		int64(len(avatarBytes)),
		currentUserAvatarContentType,
	); err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "上传群头像失败")
	}

	avatarURL, err := storageClient.PublicObjectURL(objectKey)
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "群头像存储未配置")
	}

	conversation, message, memberUserIDs, err := s.updateUserGroupConversationAvatar(user, conversationID, avatarURL)
	if err != nil {
		return groupConversationAvatarFailure(c, err)
	}

	membersByConversationID, usersByID, appsByID, err := s.loadConversationListMembers([]string{conversation.ID})
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	messageResponse := newMessageResponse(message)
	s.realtime.SendToUsers(memberUserIDs, realtimeMessageCreatedEvent(messageResponse))

	return success(c, http.StatusOK, updateGroupConversationAvatarResponse{
		Conversation: newConversationListItemResponse(
			conversation,
			user.ID,
			membersByConversationID[conversation.ID],
			usersByID,
			appsByID,
		),
		Message: messageResponse,
	})
}

func (s *Server) requireGroupConversationAvatarUpdatePermission(userID string, conversationID string) error {
	var conversation store.Conversation
	if err := s.db.First(&conversation, "id = ?", conversationID).Error; err != nil {
		return err
	}
	if conversation.Status != store.ConversationStatusActive {
		return errConversationAccessDenied
	}
	if conversation.Kind != store.ConversationKindGroup {
		return errConversationNotGroup
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
			return errConversationAccessDenied
		}
		return err
	}
	if !canManageGroupConversation(member.Role) {
		return errGroupConversationAvatarForbidden
	}

	return nil
}

func groupConversationAvatarFailure(c echo.Context, err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return failure(c, http.StatusNotFound, "not_found", "会话不存在")
	}
	if errors.Is(err, errConversationAccessDenied) {
		return failure(c, http.StatusForbidden, "forbidden", "无权访问会话")
	}
	if errors.Is(err, errConversationNotGroup) {
		return failure(c, http.StatusBadRequest, "invalid_request", "只能修改群聊头像")
	}
	if errors.Is(err, errGroupConversationAvatarForbidden) {
		return failure(c, http.StatusForbidden, "forbidden", "只有群主或管理员可以修改群头像")
	}

	return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
}

func buildGroupConversationAvatarObjectKey(conversationID string, avatarID string) string {
	return fmt.Sprintf("avatars/conversations/%s/%s.webp", strings.TrimSpace(conversationID), strings.TrimSpace(avatarID))
}
