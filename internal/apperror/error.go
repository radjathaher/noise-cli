package apperror

import "fmt"

type Error struct {
	Cause   error
	Code    string
	Exit    int
	Message string
}

func (err *Error) Error() string {
	if err.Message != "" {
		return err.Message
	}
	if err.Cause != nil {
		return err.Cause.Error()
	}

	return err.Code
}

func (err *Error) Unwrap() error {
	return err.Cause
}

func New(code string, exit int, format string, args ...any) *Error {
	return &Error{Code: code, Exit: exit, Message: fmt.Sprintf(format, args...)}
}
