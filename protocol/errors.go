package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Problem types: the RFC 9457 "type" of a standard error payload.
const (
	ErrorTypeBase         = "https://chanx.dev/errors/"
	ErrorValidationFailed = ErrorTypeBase + "validation-failed"
	ErrorInvalidJSON      = ErrorTypeBase + "invalid-json"
	ErrorHandlerFailed    = ErrorTypeBase + "handler-failed"
	ErrorInternal         = ErrorTypeBase + "internal-error"
	ErrorUnknownTopic     = ErrorTypeBase + "unknown-topic"
	ErrorUnauthorized     = ErrorTypeBase + "unauthorized"
	ErrorNotSubscribed    = ErrorTypeBase + "not-subscribed"
)

// ErrorTitles holds the fixed title of each problem type.
var ErrorTitles = map[string]string{
	ErrorValidationFailed: "Message failed validation",
	ErrorInvalidJSON:      "Frame is not valid JSON",
	ErrorHandlerFailed:    "Message handler failed",
	ErrorInternal:         "Message could not be processed",
	ErrorUnknownTopic:     "Unknown topic",
	ErrorUnauthorized:     "Subscription not allowed",
	ErrorNotSubscribed:    "Not subscribed to topic",
}

// TopicErrorTypes maps a topic error reason to its problem type.
var TopicErrorTypes = map[string]string{
	ReasonUnknownTopic:  ErrorUnknownTopic,
	ReasonUnauthorized:  ErrorUnauthorized,
	ReasonNotSubscribed: ErrorNotSubscribed,
}

// ProblemDetails is the standard error payload, an RFC 9457 problem details object.
type ProblemDetails struct {
	Type     string              `json:"type"`
	Title    string              `json:"title"`
	Detail   string              `json:"detail"`
	Status   int                 `json:"status,omitempty"`
	Instance string              `json:"instance,omitempty"`
	Errors   []ValidationFailure `json:"errors,omitempty"`
}

// ValidationFailure is one entry of ProblemDetails.Errors: the JSON Schema keyword
// that failed, a JSON Pointer to the failing value, and for "required" the missing member.
type ValidationFailure struct {
	Keyword  string `json:"keyword"`
	Pointer  string `json:"pointer"`
	Detail   string `json:"detail"`
	Property any    `json:"property,omitempty"`
}

// NewProblem builds a problem of a known type.
func NewProblem(typ, detail string, errs ...ValidationFailure) ProblemDetails {
	return ProblemDetails{Type: typ, Title: ErrorTitles[typ], Detail: detail, Errors: errs}
}

// ValidationProblem builds the validation-failed problem for errs.
func ValidationProblem(errs []ValidationFailure) ProblemDetails {
	detail := fmt.Sprintf("%d validation errors", len(errs))
	if len(errs) == 1 {
		detail = "1 validation error"
	}
	return NewProblem(ErrorValidationFailed, detail, errs...)
}

// ParseProblem reads an error frame's payload; false when it is not a problem,
// such as a server using the Pydantic format.
func ParseProblem(payload []byte) (ProblemDetails, bool) {
	var p ProblemDetails
	if err := json.Unmarshal(payload, &p); err != nil || p.Type == "" || p.Title == "" {
		return ProblemDetails{}, false
	}
	return p, true
}

// JSONPointer encodes path segments as an RFC 6901 JSON Pointer.
func JSONPointer(path []any) string {
	var b strings.Builder
	for _, seg := range path {
		s := fmt.Sprint(seg)
		s = strings.ReplaceAll(s, "~", "~0")
		s = strings.ReplaceAll(s, "/", "~1")
		b.WriteString("/" + s)
	}
	return b.String()
}

// InstancePath follows loc through data, keeping only segments that exist there,
// as every chanx implementation does: validators add labels to a location (a
// union member, a discriminator tag) that name nothing in the frame. When the last
// segment is absent from an object it is returned as the missing member.
func InstancePath(loc []any, data any) (path []any, missing any) {
	path = []any{}
	node := data
	for i, seg := range loc {
		switch n := node.(type) {
		case map[string]any:
			if key, ok := seg.(string); ok {
				if v, has := n[key]; has {
					node = v
					path = append(path, seg)
					continue
				}
			}
			if i == len(loc)-1 {
				return path, seg
			}
		case []any:
			if idx, ok := seg.(int); ok && idx >= 0 && idx < len(n) {
				node = n[idx]
				path = append(path, idx)
			}
		}
	}
	return path, nil
}
