package apperror

import "net/http"

type Code string

const (
	CodeInternal       Code = "INTERNAL"
	CodeInvalidRequest Code = "INVALID_REQUEST"
	CodeUnauthorized   Code = "UNAUTHORIZED"
	CodeForbidden      Code = "FORBIDDEN"
	CodeNotFound       Code = "NOT_FOUND"
)

type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"-"`
}

func (e *Error) Error() string {
	return e.Message
}

func New(code Code, message string, status int) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Status:  status,
	}
}

func InvalidRequest(message string) *Error {
	return New(CodeInvalidRequest, message, http.StatusBadRequest)
}

func Unauthorized(message string) *Error {
	return New(CodeUnauthorized, message, http.StatusUnauthorized)
}

func Forbidden(message string) *Error {
	return New(CodeForbidden, message, http.StatusForbidden)
}

func NotFound(message string) *Error {
	return New(CodeNotFound, message, http.StatusNotFound)
}

func Internal(message string) *Error {
	return New(CodeInternal, message, http.StatusInternalServerError)
}
