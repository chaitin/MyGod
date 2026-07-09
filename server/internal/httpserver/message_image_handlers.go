package httpserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"app/internal/store"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

const (
	imageMessageContentType     = "image/webp"
	maxImageMessageUploadBytes  = 5 * 1024 * 1024
	maxImageMessageRequestBytes = maxImageMessageUploadBytes + 1*1024*1024
	maxImageMessageDimension    = 1920
	messageTypeImage            = "image"
)

var errImageMessageTooLarge = errors.New("image message too large")

type imageMessageBody struct {
	Type   string `json:"type"`
	FileID string `json:"file_id"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

// createConversationImageMessage godoc
//
// @Summary 发送图片消息
// @Description 普通用户上传 WebP 图片并发送为会话图片消息。图片写入 temporary bucket，消息 body 只保存 file_id。
// @Tags 客户端消息
// @Accept multipart/form-data
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Param client_message_id formData string true "客户端消息 ID"
// @Param image formData file true "WebP 图片"
// @Success 200 {object} successEnvelope{data=createMessageResponse}
// @Success 201 {object} successEnvelope{data=createMessageResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 413 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/{conversation_id}/messages/images [post]
func (s *Server) createConversationImageMessage(c echo.Context) error {
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

	c.Request().Body = http.MaxBytesReader(c.Response().Writer, c.Request().Body, maxImageMessageRequestBytes)
	fileHeader, err := c.FormFile("image")
	if err != nil {
		if isRequestBodyTooLarge(err) {
			return failure(c, http.StatusRequestEntityTooLarge, "request_too_large", "图片不能超过 5MiB")
		}
		return failure(c, http.StatusBadRequest, "invalid_request", "请选择要发送的图片")
	}
	if fileHeader.Size > maxImageMessageUploadBytes {
		return failure(c, http.StatusRequestEntityTooLarge, "request_too_large", "图片不能超过 5MiB")
	}
	if fileHeader.Size <= 0 {
		return failure(c, http.StatusBadRequest, "invalid_request", "图片不能为空")
	}

	file, err := fileHeader.Open()
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", "读取图片失败")
	}
	defer file.Close()

	imageBytes, err := readImageMessageUpload(file)
	if err != nil {
		if errors.Is(err, errImageMessageTooLarge) {
			return failure(c, http.StatusRequestEntityTooLarge, "request_too_large", "图片不能超过 5MiB")
		}
		return failure(c, http.StatusBadRequest, "invalid_request", "读取图片失败")
	}
	width, height, err := parseWebPDimensions(imageBytes)
	if err != nil {
		return failure(c, http.StatusBadRequest, "invalid_request", "图片必须是 WebP 格式")
	}
	if width > maxImageMessageDimension || height > maxImageMessageDimension {
		return failure(c, http.StatusBadRequest, "invalid_request", "图片最大宽高不能超过 1920px")
	}

	storageClient, err := s.newObjectStoreClient(c.Request().Context())
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "临时文件存储未配置")
	}

	now := time.Now().UTC()
	fileID := uuid.NewString()
	objectKey := buildTemporaryObjectKey(now, fileID)
	if err := storageClient.PutTemporaryObject(
		c.Request().Context(),
		objectKey,
		bytes.NewReader(imageBytes),
		int64(len(imageBytes)),
		imageMessageContentType,
	); err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "上传图片失败")
	}

	temporaryFile := store.TemporaryFile{
		ID:        fileID,
		ObjectKey: objectKey,
		SizeBytes: int64(len(imageBytes)),
		CreatedAt: now,
	}
	if err := s.db.Create(&temporaryFile).Error; err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "保存图片失败")
	}

	body, err := json.Marshal(imageMessageBody{
		Type:   messageTypeImage,
		FileID: temporaryFile.ID,
		Width:  width,
		Height: height,
	})
	if err != nil {
		return failure(c, http.StatusInternalServerError, "internal_error", "服务端错误")
	}

	message, created, memberUserIDs, err := s.createUserMessageWithMetadata(
		c.Request().Context(),
		user.ID,
		conversationID,
		clientMessageID,
		body,
		staticMessageBodyFinalizer(imageMessageSummary()),
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

func readImageMessageUpload(reader io.Reader) ([]byte, error) {
	content, err := io.ReadAll(io.LimitReader(reader, maxImageMessageUploadBytes+1))
	if err != nil {
		return nil, err
	}
	if len(content) > maxImageMessageUploadBytes {
		return nil, errImageMessageTooLarge
	}
	if len(content) == 0 {
		return nil, errors.New("empty image")
	}

	return content, nil
}

func imageMessageSummary() string {
	return "[图片]"
}
