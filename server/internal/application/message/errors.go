package message

import "errors"

type ErrorCode string

const (
	CodeInvalidRequest     ErrorCode = "invalid_request"
	CodeForbidden          ErrorCode = "forbidden"
	CodeNotFound           ErrorCode = "not_found"
	CodeConflict           ErrorCode = "conflict"
	CodeRequestTooLarge    ErrorCode = "request_too_large"
	CodeSourceUnavailable  ErrorCode = "source_unavailable"
	CodeUnsupportedMessage ErrorCode = "unsupported_message"
	CodeContentUnavailable ErrorCode = "content_unavailable"
	CodeInternal           ErrorCode = "internal_error"
)

type Error struct {
	Code    ErrorCode
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func ErrorCodeOf(err error) ErrorCode {
	var messageErr *Error
	if errors.As(err, &messageErr) {
		return messageErr.Code
	}
	return CodeInternal
}

func ErrorMessage(err error) string {
	var messageErr *Error
	if errors.As(err, &messageErr) && messageErr.Message != "" {
		return messageErr.Message
	}
	return "服务端错误"
}

func InvalidRequestError(message string, cause error) error {
	return &Error{Code: CodeInvalidRequest, Message: message, Cause: cause}
}

func NotFoundError(message string, cause error) error {
	return &Error{Code: CodeNotFound, Message: message, Cause: cause}
}

func conflict(message string, cause error) error {
	return &Error{Code: CodeConflict, Message: message, Cause: cause}
}

func InternalError(cause error) error {
	return &Error{Code: CodeInternal, Message: "服务端错误", Cause: cause}
}

func forbidden(message string, cause error) error {
	return &Error{Code: CodeForbidden, Message: message, Cause: cause}
}

func internalError(cause error) error {
	return InternalError(cause)
}

var (
	errConversationAccessDenied  = errors.New("conversation access denied")
	errConversationNotSendable   = errors.New("conversation not sendable")
	errAppDirectAccessDenied     = errors.New("app direct access denied")
	errReplyToMessageInvalid     = errors.New("reply_to_message_id invalid")
	ErrForwardUnsupportedMessage = errors.New("forward unsupported message")
)
