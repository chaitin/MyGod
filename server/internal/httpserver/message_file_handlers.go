package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"path"
	"strings"
	"time"

	"app/internal/store"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

const (
	maxFileMessageNameLength = 255
	messageTypeFile          = "file"
)

type fileMessageBody struct {
	Type      string `json:"type"`
	FileID    string `json:"file_id"`
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
}

// createConversationFileMessage godoc
//
// @Summary 发送文件消息
// @Description 普通用户上传文件并发送为会话文件消息。文件写入 temporary bucket，消息 body 保存 file_id、文件名和文件大小。
// @Tags 客户端消息
// @Accept multipart/form-data
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Param client_message_id formData string true "客户端消息 ID"
// @Param file formData file true "文件"
// @Success 200 {object} successEnvelope{data=createMessageResponse}
// @Success 201 {object} successEnvelope{data=createMessageResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 413 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/{conversation_id}/messages/files [post]
func (s *Server) createConversationFileMessage(c echo.Context) error {
	user, ok := currentUser(c)
	if !ok {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	conversationID, err := normalizeMessageConversationID(c.Param("conversation_id"))
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}
	clientMessageID, err := normalizeClientMessageID(c.FormValue("client_message_id"))
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}
	replyToMessageID, err := normalizeOptionalMessageID(c.FormValue("reply_to_message_id"), "引用消息 ID")
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}
	existingMessage, ok, member, err := s.findExistingUserMessageBeforeFileUpload(user.ID, conversationID, clientMessageID)
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
	if ok {
		messageResponse, err := s.newMessageResponseForUser(c.Request().Context(), existingMessage, user.ID)
		if err != nil {
			return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
		}
		return success(c, http.StatusOK, createMessageResponse{
			Message: messageResponse,
		})
	}
	if err := validateReplyToMessage(s.db, conversationID, member.HistoryVisibleFromSeq, replyToMessageID); err != nil {
		if errors.Is(err, errReplyToMessageInvalid) {
			return failure(c, http.StatusBadRequest, "invalid_request", "引用消息无效")
		}
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	c.Request().Body = http.MaxBytesReader(c.Response().Writer, c.Request().Body, maxTemporaryFileUploadBytes)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		if isRequestBodyTooLarge(err) {
			return failure(c, http.StatusRequestEntityTooLarge, "request_too_large", "文件不能超过 20MiB")
		}
		return failure(c, http.StatusBadRequest, "invalid_request", "请选择要发送的文件")
	}
	if fileHeader.Size > maxTemporaryFileUploadBytes {
		return failure(c, http.StatusRequestEntityTooLarge, "request_too_large", "文件不能超过 20MiB")
	}
	if fileHeader.Size <= 0 {
		return failure(c, http.StatusBadRequest, "invalid_request", "文件不能为空")
	}
	fileName, err := normalizeFileMessageName(fileHeader.Filename)
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", err.Error())
	}

	file, err := fileHeader.Open()
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", "读取文件失败")
	}
	defer file.Close()

	storageClient, err := s.newObjectStoreClient(c.Request().Context())
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "临时文件存储未配置")
	}

	now := time.Now().UTC()
	fileID := uuid.NewString()
	objectKey := buildTemporaryObjectKey(now, fileID)
	contentType := strings.TrimSpace(fileHeader.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if err := storageClient.PutTemporaryObject(c.Request().Context(), objectKey, file, fileHeader.Size, contentType); err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "上传文件失败")
	}

	temporaryFile := store.TemporaryFile{
		ID:        fileID,
		ObjectKey: objectKey,
		SizeBytes: fileHeader.Size,
		CreatedAt: now,
	}
	if err := s.db.Create(&temporaryFile).Error; err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "保存文件失败")
	}

	body, err := json.Marshal(fileMessageBody{
		Type:      messageTypeFile,
		FileID:    temporaryFile.ID,
		Name:      fileName,
		SizeBytes: temporaryFile.SizeBytes,
	})
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	message, created, memberUserIDs, mentionedUserIDs, err := s.createUserMessageWithMetadata(
		c.Request().Context(),
		user.ID,
		conversationID,
		clientMessageID,
		body,
		staticMessageBodyFinalizer(fileMessageSummary(fileName)),
		createMessageMetadata{
			ReplyToMessageID: replyToMessageID,
			EmitAppEvent:     true,
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
		s.sendRealtimeConversationMemberMentionedToUsers(mentionedUserIDs, message)
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}

	return success(c, status, createMessageResponse{
		Message: messageResponse,
	})
}

func normalizeClientMessageID(rawClientMessageID string) (string, error) {
	clientMessageID := strings.TrimSpace(rawClientMessageID)
	if clientMessageID == "" {
		return "", errors.New("客户端消息 ID 不能为空")
	}
	if len([]rune(clientMessageID)) > maxClientMessageIDLength {
		return "", errors.New("客户端消息 ID 不能超过 128 个字符")
	}

	return clientMessageID, nil
}

func (s *Server) findExistingUserMessageBeforeFileUpload(userID string, conversationID string, clientMessageID string) (store.Message, bool, store.ConversationMember, error) {
	var conversation store.Conversation
	if err := s.db.First(&conversation, "id = ?", conversationID).Error; err != nil {
		return store.Message{}, false, store.ConversationMember{}, err
	}
	if conversation.Status != store.ConversationStatusActive ||
		conversation.PostingPolicy != store.ConversationPostingPolicyOpen {
		return store.Message{}, false, store.ConversationMember{}, errConversationNotSendable
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
			return store.Message{}, false, store.ConversationMember{}, errConversationAccessDenied
		}
		return store.Message{}, false, store.ConversationMember{}, err
	}

	existing, ok, err := findExistingMessageByClientMessageID(
		s.db,
		conversationID,
		store.MessageSenderTypeUser,
		userID,
		clientMessageID,
	)
	if err != nil {
		return store.Message{}, false, store.ConversationMember{}, err
	}
	if ok {
		if err := advanceConversationMemberReadSeq(s.db, conversationID, userID, existing.Seq); err != nil {
			return store.Message{}, false, store.ConversationMember{}, err
		}
		return existing, true, member, nil
	}

	return store.Message{}, false, member, nil
}

func normalizeFileMessageName(rawName string) (string, error) {
	name := strings.TrimSpace(path.Base(strings.ReplaceAll(rawName, "\\", "/")))
	if name == "" || name == "." || name == "/" {
		return "", errors.New("文件名不能为空")
	}
	if len([]rune(name)) > maxFileMessageNameLength {
		return "", errors.New("文件名不能超过 255 个字符")
	}

	return name, nil
}

func normalizeSpecifiedFileMessageName(rawName string) (string, error) {
	name := strings.TrimSpace(rawName)
	if name == "" || name == "." || name == "/" {
		return "", errors.New("文件名不能为空")
	}
	if strings.ContainsAny(name, `/\`) {
		return "", errors.New("文件名不能包含路径")
	}
	if len([]rune(name)) > maxFileMessageNameLength {
		return "", errors.New("文件名不能超过 255 个字符")
	}

	return name, nil
}

func fileMessageSummary(name string) string {
	return "[文件] " + strings.TrimSpace(name)
}
