package apperror

import (
	"errors"
	"net/http"
)

type Error struct {
	code       int
	message    string
	httpStatus int
	cause      error
}

func (appError *Error) Error() string {
	return appError.message
}

func (appError *Error) Unwrap() error {
	return appError.cause
}

func New(code int, message string) *Error {
	return NewWithHTTP(code, message, http.StatusOK)
}

func NewWithHTTP(code int, message string, httpStatus int) *Error {
	return &Error{code: code, message: message, httpStatus: httpStatus}
}

func WithCause(base *Error, cause error) *Error {
	return &Error{code: base.code, message: base.message, httpStatus: base.httpStatus, cause: cause}
}

func (appError *Error) Code() int {
	return appError.code
}

func (appError *Error) Message() string {
	return appError.message
}

func (appError *Error) HTTPStatus() int {
	return appError.httpStatus
}

func As(err error) (*Error, bool) {
	var appError *Error
	return appError, errors.As(err, &appError)
}
