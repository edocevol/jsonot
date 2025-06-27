package jsonot

import (
	"errors"
	"fmt"
)

// ErrCode is an error code type
type ErrCode int

const (
	// Ok indicates no error
	Ok ErrCode = iota
	// InvalidParameter indicates an invalid parameter error
	InvalidParameter
	// InvalidOperation indicates an invalid operation error
	InvalidOperation
	// InvalidPathFormat indicates an invalid path format error
	InvalidPathFormat
	// InvalidPathElement indicates an invalid path element error
	InvalidPathElement
	// BadPath indicates a bad path error
	BadPath
	// UnexpectedError indicates an unexpected error
	UnexpectedError
	// ConflictSubType indicates a conflict with a subtype name
	ConflictSubType
)

// errCodeMessages maps error codes to their corresponding messages
var errCodeMessages = map[ErrCode]string{
	Ok:                 "",
	InvalidParameter:   "invalid parameter",
	InvalidOperation:   "invalid operation",
	InvalidPathFormat:  "invalid path format",
	InvalidPathElement: "invalid path element",
	BadPath:            "bad path",
	UnexpectedError:    "unexpected error",
	ConflictSubType:    "sub type name conflict with internal sub type name",
}

// Error represents a generic error in the JSON operations
type Error struct {
	err  error
	msg  string
	code ErrCode
}

// NewError creates a new Error with the given message
func NewError(code ErrCode) *Error {
	return &Error{
		code: code,
	}
}

// IsError checks if the error is of a specific type
func IsError(err error, code ErrCode) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.code == code
	}
	return false
}

// Append adds a message to the error
func (e *Error) Append(format string, args ...interface{}) *Error {
	if len(args) == 0 {
		e.msg += format
	} else {
		e.msg += fmt.Sprintf(format, args...)
	}

	return e
}

// Error returns the error message
func (e *Error) Error() string {
	if e.err != nil {
		return e.err.Error()
	}

	codeMsg := errCodeMessages[e.code]
	if e.msg == "" {
		return codeMsg
	}

	return fmt.Sprintf("%s: %s", codeMsg, e.msg)
}

// Wrap wraps the error with a message
func (e *Error) Wrap(err error) *Error {
	e.err = fmt.Errorf("%w: %w", e, err)
	return e
}
