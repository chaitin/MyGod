package account

import (
	"errors"
	"fmt"
)

type ErrorCode string

const (
	CodeInvalidRequest     ErrorCode = "invalid_request"
	CodeInvalidCredentials ErrorCode = "invalid_credentials"
	CodeUnauthorized       ErrorCode = "unauthorized"
	CodeNotFound           ErrorCode = "not_found"
	CodeConflict           ErrorCode = "conflict"
	CodeRequestTooLarge    ErrorCode = "request_too_large"
	CodeLoginUnavailable   ErrorCode = "password_login_unavailable"
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
	var accountErr *Error
	if errors.As(err, &accountErr) {
		return accountErr.Code
	}
	return CodeInternal
}

func ErrorMessage(err error) string {
	var accountErr *Error
	if errors.As(err, &accountErr) && accountErr.Message != "" {
		return accountErr.Message
	}
	return "服务端错误"
}

func newError(code ErrorCode, message string, cause error) error {
	return &Error{Code: code, Message: message, Cause: cause}
}

func internalError(cause error) error {
	return newError(CodeInternal, "服务端错误", cause)
}

func wrapInternal(message string, cause error) error {
	if cause == nil {
		cause = fmt.Errorf("%s", message)
	}
	return newError(CodeInternal, message, cause)
}
