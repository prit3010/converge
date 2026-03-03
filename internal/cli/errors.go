package cli

import (
	"errors"
	"fmt"
	"strings"
)

type ErrorCode string

const (
	ErrorCodeValidation     ErrorCode = "VALIDATION"
	ErrorCodeNotInitialized ErrorCode = "NOT_INITIALIZED"
	ErrorCodeNotFound       ErrorCode = "NOT_FOUND"
	ErrorCodeConflict       ErrorCode = "CONFLICT"
	ErrorCodeExternal       ErrorCode = "EXTERNAL"
	ErrorCodeInternal       ErrorCode = "INTERNAL"
)

type commandError struct {
	Code     ErrorCode `json:"code"`
	Message  string    `json:"message"`
	ExitCode int       `json:"exit_code"`
	cause    error
}

func (e *commandError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *commandError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func validationErrorf(format string, args ...any) *commandError {
	return newCommandError(ErrorCodeValidation, fmt.Sprintf(format, args...), nil)
}

func notInitializedErrorf(format string, args ...any) *commandError {
	return newCommandError(ErrorCodeNotInitialized, fmt.Sprintf(format, args...), nil)
}

func notFoundErrorf(format string, args ...any) *commandError {
	return newCommandError(ErrorCodeNotFound, fmt.Sprintf(format, args...), nil)
}

func conflictErrorf(format string, args ...any) *commandError {
	return newCommandError(ErrorCodeConflict, fmt.Sprintf(format, args...), nil)
}

func externalErrorf(format string, args ...any) *commandError {
	return newCommandError(ErrorCodeExternal, fmt.Sprintf(format, args...), nil)
}

func internalErrorf(format string, args ...any) *commandError {
	return newCommandError(ErrorCodeInternal, fmt.Sprintf(format, args...), nil)
}

func wrapCommandError(code ErrorCode, err error, format string, args ...any) *commandError {
	msg := fmt.Sprintf(format, args...)
	if err != nil {
		msg = fmt.Sprintf("%s: %s", msg, err.Error())
	}
	return newCommandError(code, msg, err)
}

func newCommandError(code ErrorCode, message string, cause error) *commandError {
	return &commandError{
		Code:     code,
		Message:  strings.TrimSpace(message),
		ExitCode: exitCodeForError(code),
		cause:    cause,
	}
}

func exitCodeForError(code ErrorCode) int {
	switch code {
	case ErrorCodeValidation:
		return 2
	case ErrorCodeNotInitialized:
		return 3
	case ErrorCodeNotFound:
		return 4
	case ErrorCodeConflict:
		return 5
	case ErrorCodeExternal:
		return 6
	default:
		return 1
	}
}

func classifyCommandError(err error) *commandError {
	if err == nil {
		return nil
	}
	var cmdErr *commandError
	if errors.As(err, &cmdErr) && cmdErr != nil {
		return cmdErr
	}

	text := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(text, "not a converge repository"):
		return wrapCommandError(ErrorCodeNotInitialized, err, "repository is not initialized")
	case strings.Contains(text, "missing required flag"), strings.Contains(text, "cannot use --branch and --all together"), strings.Contains(text, "invalid "), strings.Contains(text, "required"):
		return wrapCommandError(ErrorCodeValidation, err, err.Error())
	case strings.Contains(text, "not found"):
		return wrapCommandError(ErrorCodeNotFound, err, err.Error())
	case strings.Contains(text, "already exists"), strings.Contains(text, "already in progress"), strings.Contains(text, "cannot rotate while restore is in progress"):
		return wrapCommandError(ErrorCodeConflict, err, err.Error())
	case strings.Contains(text, "openai"), strings.HasPrefix(text, "git "), strings.Contains(text, "command not found"):
		return wrapCommandError(ErrorCodeExternal, err, err.Error())
	default:
		return wrapCommandError(ErrorCodeInternal, err, err.Error())
	}
}
