package client

import (
	"net/http"

	messageapp "app/internal/application/message"

	"github.com/labstack/echo/v4"
)

type forwardMessagesRequest struct {
	ClientForwardID       string   `json:"client_forward_id"`
	MessageIDs            []string `json:"message_ids"`
	Mode                  string   `json:"mode"`
	TargetConversationIDs []string `json:"target_conversation_ids"`
}

type forwardMessagesResponse struct {
	FailedCount int                           `json:"failed_count"`
	Results     []forwardMessagesTargetResult `json:"results"`
	SentCount   int                           `json:"sent_count"`
}

type forwardMessagesTargetResult struct {
	ConversationID string                      `json:"conversation_id"`
	Error          *forwardMessagesTargetError `json:"error,omitempty"`
	Messages       []messageResponse           `json:"messages,omitempty"`
	Status         string                      `json:"status"`
}

type forwardMessagesTargetError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// forward godoc
//
// @Summary 转发会话消息
// @Description 将同一源会话中的一条或多条可见消息逐条或合并转发到多个目标会话。目标会话之间允许部分成功。
// @Tags 客户端消息
// @Accept json
// @Produce json
// @Param conversation_id path string true "源会话 ID"
// @Param body body forwardMessagesRequest true "转发请求"
// @Success 200 {object} successEnvelope{data=forwardMessagesResponse}
// @Failure 400 {object} errorEnvelope
// @Failure 401 {object} errorEnvelope
// @Failure 403 {object} errorEnvelope
// @Failure 404 {object} errorEnvelope
// @Failure 409 {object} errorEnvelope
// @Failure 500 {object} errorEnvelope
// @Router /api/client/conversations/{conversation_id}/messages/forward [post]
func (a *MessageAPI) forward(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, http.StatusInternalServerError, string(messageapp.CodeInternal), "服务端错误")
	}
	sourceConversationID, err := normalizeMessageConversationID(c.Param("conversation_id"))
	if err != nil {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), err.Error())
	}
	var request forwardMessagesRequest
	if err := c.Bind(&request); err != nil {
		return writeFailure(c, http.StatusBadRequest, string(messageapp.CodeInvalidRequest), "请求格式错误")
	}
	result, err := a.messages.Forward(c.Request().Context(), messageapp.ForwardCommand{
		AccountID: current.ID, ClientForwardID: request.ClientForwardID, MessageIDs: request.MessageIDs,
		Mode: request.Mode, SourceConversationID: sourceConversationID, TargetConversationIDs: request.TargetConversationIDs,
	})
	if err != nil {
		return writeMessageError(c, err)
	}
	response := forwardMessagesResponse{
		FailedCount: result.FailedCount, Results: make([]forwardMessagesTargetResult, 0, len(result.Results)), SentCount: result.SentCount,
	}
	for _, target := range result.Results {
		converted := forwardMessagesTargetResult{ConversationID: target.ConversationID, Status: target.Status}
		if target.Error != nil {
			converted.Error = &forwardMessagesTargetError{Code: target.Error.Code, Message: target.Error.Message}
		}
		if len(target.Messages) > 0 {
			converted.Messages = make([]messageResponse, 0, len(target.Messages))
			for _, message := range target.Messages {
				converted.Messages = append(converted.Messages, newClientMessageResponse(message))
			}
		}
		response.Results = append(response.Results, converted)
	}
	return writeSuccess(c, http.StatusOK, response)
}
