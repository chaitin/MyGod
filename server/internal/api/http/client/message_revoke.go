package client

import (
	"net/http"

	messageapp "app/internal/application/message"

	"github.com/labstack/echo/v4"
)

type revokeConversationMessageResponse struct {
	Message       messageResponse `json:"message"`
	SystemMessage messageResponse `json:"system_message"`
}

// revoke godoc
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
func (a *MessageAPI) revoke(c echo.Context) error {
	current, ok := CurrentAccount(c)
	if !ok {
		return writeFailure(c, http.StatusInternalServerError, string(messageapp.CodeInternal), "服务端错误")
	}
	result, err := a.messages.Revoke(c.Request().Context(), messageapp.RevokeCommand{
		AccountID: current.ID, ConversationID: c.Param("conversation_id"), MessageID: c.Param("message_id"),
	})
	if err != nil {
		return writeMessageError(c, err)
	}
	return writeSuccess(c, http.StatusOK, revokeConversationMessageResponse{
		Message: newClientMessageResponse(result.Message), SystemMessage: newClientMessageResponse(result.SystemMessage),
	})
}
