package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"

	fileapp "app/internal/application/file"
	messageapp "app/internal/application/message"
	"app/internal/media"

	"github.com/labstack/echo/v4"
)

const (
	maxFileMessageNameLength = 255

	imageMessageContentType     = "image/webp"
	maxImageMessageUploadBytes  = 5 * 1024 * 1024
	maxImageMessageRequestBytes = maxImageMessageUploadBytes + 1*1024*1024
	maxImageMessageDimension    = 1920

	maxVoiceMessageDurationMS   = 60_000
	maxVoiceMessageUploadBytes  = 1 * 1024 * 1024
	maxVoiceMessageRequestBytes = maxVoiceMessageUploadBytes + 512*1024
	voiceMessageContentType     = "audio/webm"
	voiceMessageDemoTranscript  = "这是一段语音消息的演示转写文字"
)

var (
	errImageMessageTooLarge = errors.New("image message too large")
	errVoiceMessageTooLarge = errors.New("voice message too large")
	webMHeader              = []byte{0x1a, 0x45, 0xdf, 0xa3}
)

type fileMessageBody struct {
	Type      string `json:"type"`
	FileID    string `json:"file_id"`
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
}

type imageMessageBody struct {
	Type   string `json:"type"`
	FileID string `json:"file_id"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

type voiceMessageBody struct {
	Type        string `json:"type"`
	FileID      string `json:"file_id"`
	DurationMS  int    `json:"duration_ms"`
	SizeBytes   int64  `json:"size_bytes"`
	ContentType string `json:"content_type"`
	Transcript  string `json:"transcript"`
}

// createFile godoc
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
func (a *MessageAPI) createFile(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, http.StatusInternalServerError, string(messageapp.CodeInternal), "服务端错误")
	}
	conversationID, err := normalizeMessageConversationID(c.Param("conversation_id"))
	if err != nil {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), err.Error())
	}
	clientMessageID := c.FormValue("client_message_id")
	replyToMessageID := c.FormValue("reply_to_message_id")
	if handled, err := a.prepareAttachmentUpload(c, current.ID, conversationID, clientMessageID, replyToMessageID); handled || err != nil {
		return err
	}
	c.Request().Body = http.MaxBytesReader(c.Response().Writer, c.Request().Body, fileapp.MaxTemporaryUploadBytes)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		if isRequestBodyTooLarge(err) {
			return writeFailure(c, http.StatusRequestEntityTooLarge, string(messageapp.CodeRequestTooLarge), "文件不能超过 20MiB")
		}
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), "请选择要发送的文件")
	}
	if fileHeader.Size > fileapp.MaxTemporaryUploadBytes {
		return writeFailure(c, http.StatusRequestEntityTooLarge, string(messageapp.CodeRequestTooLarge), "文件不能超过 20MiB")
	}
	if fileHeader.Size <= 0 {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), "文件不能为空")
	}
	fileName, err := normalizeFileMessageName(fileHeader.Filename)
	if err != nil {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), err.Error())
	}
	file, err := fileHeader.Open()
	if err != nil {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), "读取文件失败")
	}
	defer file.Close()
	contentType := strings.TrimSpace(fileHeader.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	temporary, err := a.files.UploadTemporary(c.Request().Context(), fileapp.UploadTemporaryCommand{
		Content: file, ContentType: contentType, SizeBytes: fileHeader.Size,
	})
	if err != nil {
		return writeMessageFileError(c, err)
	}
	body, err := json.Marshal(fileMessageBody{Type: "file", FileID: temporary.ID, Name: fileName, SizeBytes: temporary.SizeBytes})
	if err != nil {
		return writeFailure(c, http.StatusInternalServerError, string(messageapp.CodeInternal), "服务端错误")
	}
	return a.createPreparedAttachment(c, messageapp.CreatePreparedCommand{
		AccountID: current.ID, Body: body, ClientMessageID: clientMessageID,
		ConversationID: conversationID, ReplyToMessageID: replyToMessageID,
		Summary: "[文件] " + strings.TrimSpace(fileName),
	})
}

// createImage godoc
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
func (a *MessageAPI) createImage(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, http.StatusInternalServerError, string(messageapp.CodeInternal), "服务端错误")
	}
	conversationID, err := normalizeMessageConversationID(c.Param("conversation_id"))
	if err != nil {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), err.Error())
	}
	clientMessageID := c.FormValue("client_message_id")
	replyToMessageID := c.FormValue("reply_to_message_id")
	if handled, err := a.prepareAttachmentUpload(c, current.ID, conversationID, clientMessageID, replyToMessageID); handled || err != nil {
		return err
	}
	c.Request().Body = http.MaxBytesReader(c.Response().Writer, c.Request().Body, maxImageMessageRequestBytes)
	fileHeader, err := c.FormFile("image")
	if err != nil {
		if isRequestBodyTooLarge(err) {
			return writeFailure(c, http.StatusRequestEntityTooLarge, string(messageapp.CodeRequestTooLarge), "图片不能超过 5MiB")
		}
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), "请选择要发送的图片")
	}
	if fileHeader.Size > maxImageMessageUploadBytes {
		return writeFailure(c, http.StatusRequestEntityTooLarge, string(messageapp.CodeRequestTooLarge), "图片不能超过 5MiB")
	}
	if fileHeader.Size <= 0 {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), "图片不能为空")
	}
	file, err := fileHeader.Open()
	if err != nil {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), "读取图片失败")
	}
	defer file.Close()
	content, err := readImageMessageUpload(file)
	if err != nil {
		if errors.Is(err, errImageMessageTooLarge) {
			return writeFailure(c, http.StatusRequestEntityTooLarge, string(messageapp.CodeRequestTooLarge), "图片不能超过 5MiB")
		}
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), "读取图片失败")
	}
	width, height, err := media.WebPDimensions(content)
	if err != nil {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), "图片必须是 WebP 格式")
	}
	if width > maxImageMessageDimension || height > maxImageMessageDimension {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), "图片最大宽高不能超过 1920px")
	}
	temporary, err := a.files.UploadTemporary(c.Request().Context(), fileapp.UploadTemporaryCommand{
		Content: bytes.NewReader(content), ContentType: imageMessageContentType, SizeBytes: int64(len(content)),
	})
	if err != nil {
		return writeMessageFileError(c, err)
	}
	body, err := json.Marshal(imageMessageBody{Type: "image", FileID: temporary.ID, Width: width, Height: height})
	if err != nil {
		return writeFailure(c, http.StatusInternalServerError, string(messageapp.CodeInternal), "服务端错误")
	}
	return a.createPreparedAttachment(c, messageapp.CreatePreparedCommand{
		AccountID: current.ID, Body: body, ClientMessageID: clientMessageID,
		ConversationID: conversationID, ReplyToMessageID: replyToMessageID, Summary: "[图片]",
	})
}

// createVoice godoc
//
// @Summary 发送语音消息
// @Description 普通用户上传最长 60 秒的 WebM/Opus 音频并发送为会话语音消息。音频写入 temporary bucket，消息 body 保存 file_id、时长、文件大小、内容类型和转写文字。
// @Tags 客户端消息
// @Accept multipart/form-data
// @Produce json
// @Param conversation_id path string true "会话 ID"
// @Param client_message_id formData string true "客户端消息 ID"
// @Param reply_to_message_id formData string false "引用消息 ID"
// @Param duration_ms formData int true "语音时长（毫秒，最大 60000）"
// @Param voice formData file true "WebM/Opus 语音文件"
// @Success 200 {object} successEnvelope{data=createMessageResponse}
// @Success 201 {object} successEnvelope{data=createMessageResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 413 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/{conversation_id}/messages/voices [post]
func (a *MessageAPI) createVoice(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, http.StatusInternalServerError, string(messageapp.CodeInternal), "服务端错误")
	}
	conversationID, err := normalizeMessageConversationID(c.Param("conversation_id"))
	if err != nil {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), err.Error())
	}
	c.Request().Body = http.MaxBytesReader(c.Response().Writer, c.Request().Body, maxVoiceMessageRequestBytes)
	if err := c.Request().ParseMultipartForm(maxVoiceMessageRequestBytes); err != nil {
		if isRequestBodyTooLarge(err) {
			return writeFailure(c, http.StatusRequestEntityTooLarge, string(messageapp.CodeRequestTooLarge), "语音文件不能超过 1MiB")
		}
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), "请求格式错误")
	}
	clientMessageID := c.FormValue("client_message_id")
	replyToMessageID := c.FormValue("reply_to_message_id")
	if handled, err := a.prepareAttachmentUpload(c, current.ID, conversationID, clientMessageID, replyToMessageID); handled || err != nil {
		return err
	}
	durationMS, err := normalizeVoiceMessageDuration(c.FormValue("duration_ms"))
	if err != nil {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), err.Error())
	}
	fileHeader, err := c.FormFile("voice")
	if err != nil {
		if isRequestBodyTooLarge(err) {
			return writeFailure(c, http.StatusRequestEntityTooLarge, string(messageapp.CodeRequestTooLarge), "语音文件不能超过 1MiB")
		}
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), "请选择要发送的语音")
	}
	if fileHeader.Size > maxVoiceMessageUploadBytes {
		return writeFailure(c, http.StatusRequestEntityTooLarge, string(messageapp.CodeRequestTooLarge), "语音文件不能超过 1MiB")
	}
	if fileHeader.Size <= 0 {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), "语音文件不能为空")
	}
	contentType, _, err := mime.ParseMediaType(strings.TrimSpace(fileHeader.Header.Get("Content-Type")))
	if err != nil || !strings.EqualFold(contentType, voiceMessageContentType) {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), "语音文件必须是 WebM/Opus 格式")
	}
	file, err := fileHeader.Open()
	if err != nil {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), "读取语音文件失败")
	}
	defer file.Close()
	content, err := readVoiceMessageUpload(file)
	if err != nil {
		if errors.Is(err, errVoiceMessageTooLarge) {
			return writeFailure(c, http.StatusRequestEntityTooLarge, string(messageapp.CodeRequestTooLarge), "语音文件不能超过 1MiB")
		}
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), err.Error())
	}
	temporary, err := a.files.UploadTemporary(c.Request().Context(), fileapp.UploadTemporaryCommand{
		Content: bytes.NewReader(content), ContentType: voiceMessageContentType, SizeBytes: int64(len(content)),
	})
	if err != nil {
		return writeMessageFileError(c, err)
	}
	body, err := json.Marshal(voiceMessageBody{
		Type: "voice", FileID: temporary.ID, DurationMS: durationMS, SizeBytes: temporary.SizeBytes,
		ContentType: voiceMessageContentType, Transcript: voiceMessageDemoTranscript,
	})
	if err != nil {
		return writeFailure(c, http.StatusInternalServerError, string(messageapp.CodeInternal), "服务端错误")
	}
	return a.createPreparedAttachment(c, messageapp.CreatePreparedCommand{
		AccountID: current.ID, Body: body, ClientMessageID: clientMessageID,
		ConversationID: conversationID, ReplyToMessageID: replyToMessageID,
		Summary: voiceMessageSummary(durationMS, voiceMessageDemoTranscript),
	})
}

func (a *MessageAPI) prepareAttachmentUpload(c echo.Context, accountID, conversationID, clientMessageID, replyToMessageID string) (bool, error) {
	result, err := a.messages.PrepareUpload(c.Request().Context(), messageapp.PrepareUploadCommand{
		AccountID: accountID, ConversationID: conversationID,
		ClientMessageID: clientMessageID, ReplyToMessageID: replyToMessageID,
	})
	if err != nil {
		return true, writeMessageError(c, err)
	}
	if result.Existing == nil {
		return false, nil
	}
	return true, writeSuccess(c, http.StatusOK, createMessageResponse{Message: newClientMessageResponse(*result.Existing)})
}

func (a *MessageAPI) createPreparedAttachment(c echo.Context, command messageapp.CreatePreparedCommand) error {
	result, err := a.messages.CreatePrepared(c.Request().Context(), command)
	if err != nil {
		return writeMessageError(c, err)
	}
	status := http.StatusOK
	if result.Created {
		status = http.StatusCreated
	}
	return writeSuccess(c, status, createMessageResponse{Message: newClientMessageResponse(result.Message)})
}

func writeMessageFileError(c echo.Context, err error) error {
	if fileapp.ErrorCodeOf(err) == fileapp.CodeStorageUnavailable {
		return writeFailure(c, http.StatusInternalServerError, string(messageapp.CodeInternal), "临时文件存储未配置")
	}
	return writeFailure(c, http.StatusInternalServerError, string(messageapp.CodeInternal), fileapp.ErrorMessage(err))
}

func normalizeFileMessageName(raw string) (string, error) {
	name := strings.TrimSpace(path.Base(strings.ReplaceAll(raw, "\\", "/")))
	if name == "" || name == "." || name == "/" {
		return "", errors.New("文件名不能为空")
	}
	if len([]rune(name)) > maxFileMessageNameLength {
		return "", errors.New("文件名不能超过 255 个字符")
	}
	return name, nil
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

func normalizeVoiceMessageDuration(raw string) (int, error) {
	duration, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || duration <= 0 {
		return 0, errors.New("语音时长必须是正整数")
	}
	if duration > maxVoiceMessageDurationMS {
		return 0, errors.New("语音时长不能超过 60 秒")
	}
	return duration, nil
}

func readVoiceMessageUpload(reader io.Reader) ([]byte, error) {
	content, err := io.ReadAll(io.LimitReader(reader, maxVoiceMessageUploadBytes+1))
	if err != nil {
		return nil, errors.New("读取语音文件失败")
	}
	if len(content) > maxVoiceMessageUploadBytes {
		return nil, errVoiceMessageTooLarge
	}
	if len(content) == 0 {
		return nil, errors.New("语音文件不能为空")
	}
	if !bytes.HasPrefix(content, webMHeader) || !bytes.Contains(content, []byte("webm")) || !bytes.Contains(content, []byte("OpusHead")) {
		return nil, errors.New("语音文件必须是 WebM/Opus 格式")
	}
	return content, nil
}

func voiceMessageSummary(durationMS int, transcript string) string {
	totalSeconds := (durationMS + 999) / 1000
	summary := fmt.Sprintf("[语音] %02d:%02d", totalSeconds/60, totalSeconds%60)
	transcript = strings.TrimSpace(transcript)
	if transcript == "" {
		return summary
	}
	return summary + " - " + transcript
}
