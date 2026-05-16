package contracterr

import "strings"

const (
	CodeUnauthenticated  = "UNAUTHENTICATED"
	CodeForbidden        = "FORBIDDEN"
	CodeValidationFailed = "VALIDATION_FAILED"
	CodeNotFound         = "NOT_FOUND"
	CodeConflict         = "CONFLICT"
	CodeUnsupported      = "UNSUPPORTED"
	CodeEntryConflict    = "ENTRY_CONFLICT"
	CodeInternal         = "INTERNAL"
)

type Error struct {
	Code    string
	Message string
	Fields  map[string]string
	Err     error
}

func (err *Error) Error() string {
	if err == nil {
		return ""
	}
	if strings.TrimSpace(err.Message) != "" {
		return err.Message
	}
	if err.Err != nil {
		return err.Err.Error()
	}
	return err.Code
}

func (err *Error) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Err
}

func Validation(message string, fields map[string]string) error {
	if strings.TrimSpace(message) == "" {
		message = "validation failed"
	}
	return &Error{Code: CodeValidationFailed, Message: message, Fields: fields}
}

func Conflict(message string, cause error) error {
	if strings.TrimSpace(message) == "" {
		message = "resource conflict"
	}
	return &Error{Code: CodeConflict, Message: message, Err: cause}
}

func Unsupported(message string) error {
	if strings.TrimSpace(message) == "" {
		message = "operation is not supported"
	}
	return &Error{Code: CodeUnsupported, Message: message}
}
