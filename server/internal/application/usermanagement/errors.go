package usermanagement

import "errors"

type ErrorCode string

const (
	CodeInvalidRequest ErrorCode = "invalid_request"
	CodeNotFound       ErrorCode = "not_found"
	CodeConflict       ErrorCode = "conflict"
	CodeInternal       ErrorCode = "internal_error"
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
	var userErr *Error
	if errors.As(err, &userErr) {
		return userErr.Code
	}
	return CodeInternal
}

func ErrorMessage(err error) string {
	var userErr *Error
	if errors.As(err, &userErr) && userErr.Message != "" {
		return userErr.Message
	}
	return "服务端错误"
}

func newError(code ErrorCode, message string, cause error) error {
	return &Error{Code: code, Message: message, Cause: cause}
}

func internalError(cause error) error {
	return newError(CodeInternal, "服务端错误", cause)
}
