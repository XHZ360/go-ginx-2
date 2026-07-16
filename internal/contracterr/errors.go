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
	CodeProviderNotReady = "PROVIDER_NOT_READY"
	// CodeConfirmationRequired 表示高风险操作（如删除正在服务且已绑定的证书）需要调用方提供匹配的强确认（ConfirmHost/ConfirmCertificateID）。
	CodeConfirmationRequired = "CONFIRMATION_REQUIRED"
	// CodeCertificateIncompatible 表示证书与 Domain/代理 SNI 主机不兼容。
	CodeCertificateIncompatible = "CERTIFICATE_INCOMPATIBLE"
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

func ProviderNotReady(message string, fields map[string]string) error {
	if strings.TrimSpace(message) == "" {
		message = "certificate provider is not ready"
	}
	return &Error{Code: CodeProviderNotReady, Message: message, Fields: fields}
}

// ConfirmationRequired 构造一个表示需要强确认的可消费错误。fields 可携带需要确认的字段提示（如 confirmHost、confirmCertificateId）。
func ConfirmationRequired(message string, fields map[string]string) error {
	if strings.TrimSpace(message) == "" {
		message = "confirmation is required"
	}
	return &Error{Code: CodeConfirmationRequired, Message: message, Fields: fields}
}

// CertificateIncompatible 构造一个表示证书与 Domain/代理不兼容（主机不覆盖）的可消费错误。
func CertificateIncompatible(message string, fields map[string]string) error {
	if strings.TrimSpace(message) == "" {
		message = "certificate is incompatible"
	}
	return &Error{Code: CodeCertificateIncompatible, Message: message, Fields: fields}
}
