package parse

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityNote    Severity = "note"
)

const (
	CodeStatusInFrontmatter  = "TKT-W001"
	CodeMissingID            = "TKT-W002"
	CodeMissingKind          = "TKT-W003"
	CodeMissingTitle         = "TKT-W004"
	CodeUnexpectedFieldType  = "TKT-W005"
	CodeMissingActor         = "TKT-W006"
	CodeDuplicateKey         = "TKT-W007"
	CodeEventNameNotULID     = "TKT-W008"
	CodeUnknownStatus        = "TKT-W009"
	CodeDuplicateEventID     = "TKT-W010"
	CodeMalformedFrontmatter = "TKT-E001"
	CodeUnterminatedFrontmtr = "TKT-E002"
	CodeMissingEventType     = "TKT-E003"
	CodeInvalidEventTime     = "TKT-E004"
	CodeMissingRequiredField = "TKT-E005"
	CodeUnknownEventType     = "TKT-N001"
	CodeEventSkipped         = "TKT-N002"
	CodeConcurrentStatus     = "TKT-N003"
)

type Diagnostic struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Line     int      `json:"line,omitempty"`
	Field    string   `json:"field,omitempty"`
}

func severityOf(code string) Severity {
	switch code[4] {
	case 'E':
		return SeverityError
	case 'W':
		return SeverityWarning
	}
	return SeverityNote
}

func diag(code, message string, line int, field string) Diagnostic {
	return Diagnostic{Code: code, Severity: severityOf(code), Message: message, Line: line, Field: field}
}

func diagAt(sev Severity, code, message string, line int, field string) Diagnostic {
	return Diagnostic{Code: code, Severity: sev, Message: message, Line: line, Field: field}
}
