package contact

import "errors"

type ErrorCode string

const CodeInternal ErrorCode = "internal_error"

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
	var contactErr *Error
	if errors.As(err, &contactErr) {
		return contactErr.Code
	}
	return CodeInternal
}

func ErrorMessage(err error) string {
	var contactErr *Error
	if errors.As(err, &contactErr) && contactErr.Message != "" {
		return contactErr.Message
	}
	return "服务端错误"
}

func internalError(cause error) error {
	return &Error{Code: CodeInternal, Message: "服务端错误", Cause: cause}
}
