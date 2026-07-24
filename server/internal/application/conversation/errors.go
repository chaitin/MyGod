package conversation

import "errors"

type ErrorCode string

const (
	CodeInvalidRequest  ErrorCode = "invalid_request"
	CodeForbidden       ErrorCode = "forbidden"
	CodeNotFound        ErrorCode = "not_found"
	CodeConflict        ErrorCode = "conflict"
	CodeRequestTooLarge ErrorCode = "request_too_large"
	CodeInternal        ErrorCode = "internal_error"
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
	var conversationErr *Error
	if errors.As(err, &conversationErr) {
		return conversationErr.Code
	}
	return CodeInternal
}

func ErrorMessage(err error) string {
	var conversationErr *Error
	if errors.As(err, &conversationErr) && conversationErr.Message != "" {
		return conversationErr.Message
	}
	return "服务端错误"
}

func newError(code ErrorCode, message string, cause error) error {
	return &Error{Code: code, Message: message, Cause: cause}
}

func invalidRequest(message string, cause error) error {
	return newError(CodeInvalidRequest, message, cause)
}

func forbidden(message string, cause error) error {
	return newError(CodeForbidden, message, cause)
}

func notFound(message string, cause error) error {
	return newError(CodeNotFound, message, cause)
}

func conflict(message string, cause error) error {
	return newError(CodeConflict, message, cause)
}

func internalError(cause error) error {
	return newError(CodeInternal, "服务端错误", cause)
}

var (
	ErrAccessDenied            = errors.New("conversation access denied")
	ErrNotGroup                = errors.New("conversation is not group")
	ErrMemberCap               = errors.New("group conversation member cap exceeded")
	ErrMemberMissing           = errors.New("group conversation member missing")
	ErrGroupAppUnavailable     = errors.New("group conversation application is unavailable")
	ErrAppInviteForbidden      = errors.New("group conversation application invite forbidden")
	ErrAvatarForbidden         = errors.New("group conversation avatar forbidden")
	ErrOwnerCannotLeave        = errors.New("group conversation owner cannot leave")
	ErrOwnerCannotRemove       = errors.New("group conversation owner cannot be removed")
	ErrCannotRemoveSelf        = errors.New("group conversation member cannot remove self")
	ErrProjectInvalid          = errors.New("invalid group conversation project")
	ErrProjectPersonal         = errors.New("personal project cannot link group conversation")
	ErrProjectUnowned          = errors.New("group conversation project is not owned by user")
	ErrProjectMissing          = errors.New("group conversation project missing")
	ErrProjectMutation         = errors.New("group conversation project mutation failed")
	ErrProjectLockChange       = errors.New("group conversation project lock set changed")
	ErrProjectDissolveConflict = errors.New("group conversation project dissolution conflict")
	ErrTopicInvalidSource      = errors.New("invalid topic source message")
	ErrTopicNested             = errors.New("nested topic is not allowed")
	ErrTopicArchived           = errors.New("topic is archived")
	ErrTopicChanged            = errors.New("topic changed")
	ErrBuiltinAssistantPin     = errors.New("builtin assistant pin is immutable")
)
