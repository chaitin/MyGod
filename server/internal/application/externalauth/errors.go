package externalauth

import "errors"

type ErrorCode string

const (
	CodeInvalidRequest         ErrorCode = "invalid_request"
	CodeNotFound               ErrorCode = "not_found"
	CodeInvalidThirdPartyLogin ErrorCode = "invalid_third_party_login"
	CodeInvalidCredentials     ErrorCode = "invalid_credentials"
	CodeConflict               ErrorCode = "conflict"
	CodeInternal               ErrorCode = "internal_error"
)

type Error struct {
	Code         ErrorCode
	Message      string
	Cause        error
	oauthFailure bool
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
	var externalErr *Error
	if errors.As(err, &externalErr) {
		return externalErr.Code
	}
	return CodeInternal
}

func ErrorMessage(err error) string {
	var externalErr *Error
	if errors.As(err, &externalErr) && externalErr.Message != "" {
		return externalErr.Message
	}
	return "服务端错误"
}

func newError(code ErrorCode, message string, cause error) error {
	return &Error{Code: code, Message: message, Cause: cause}
}

func internalError(cause error) error {
	return newError(CodeInternal, "服务端错误", cause)
}

func oauthFailure(cause error) error {
	return &Error{Code: CodeInvalidThirdPartyLogin, Message: "第三方登录失败", Cause: cause, oauthFailure: true}
}

func IsOAuthFailure(err error) bool {
	var externalErr *Error
	return errors.As(err, &externalErr) && externalErr.oauthFailure
}
