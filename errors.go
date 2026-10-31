package chanx

import (
	"strings"

	"go.chanx.dev/v2/protocol"
)

// ErrorFormat is the shape of error payloads (error_format in Python).
type ErrorFormat string

const (
	// StandardErrors sends RFC 9457 problem details, the cross-language format.
	StandardErrors ErrorFormat = "standard"
	// PydanticErrors sends chanx's earlier payloads: Pydantic's error list,
	// {detail} or {reason, detail}.
	PydanticErrors ErrorFormat = "pydantic"
)

// keywords maps Pydantic error types, which Issue carries, to the JSON Schema
// keyword that fails. Kept identical to chanx's chanx/core/errors.py.
var keywords = map[string]string{
	"missing":                    "required",
	"union_tag_not_found":        "required",
	"union_tag_invalid":          "enum",
	"literal_error":              "enum",
	"enum":                       "enum",
	"string_too_short":           "minLength",
	"string_too_long":            "maxLength",
	"string_pattern_mismatch":    "pattern",
	"greater_than_equal":         "minimum",
	"greater_than":               "exclusiveMinimum",
	"less_than_equal":            "maximum",
	"less_than":                  "exclusiveMaximum",
	"multiple_of":                "multipleOf",
	"too_short":                  "minItems",
	"too_long":                   "maxItems",
	"extra_forbidden":            "additionalProperties",
	"uuid_parsing":               "format",
	"uuid_version":               "format",
	"url_parsing":                "format",
	"url_scheme":                 "format",
	"url_syntax_violation":       "format",
	"url_too_long":               "format",
	"date_parsing":               "format",
	"date_from_datetime_parsing": "format",
	"datetime_parsing":           "format",
	"datetime_from_date_parsing": "format",
	"time_parsing":               "format",
	"time_delta_parsing":         "format",
}

func keywordFor(issueType string) string {
	if k, ok := keywords[issueType]; ok {
		return k
	}
	for _, suffix := range []string{"_type", "_parsing", "_from_float"} {
		if strings.HasSuffix(issueType, suffix) {
			return "type"
		}
	}
	if issueType == "none_required" {
		return "type"
	}
	return "custom"
}

func validationPayload(format ErrorFormat, issues []Issue, data any, camelize bool) any {
	if format == PydanticErrors {
		return issues
	}
	if len(issues) > 0 && issues[0].Type == "json_invalid" {
		return protocol.NewProblem(protocol.ErrorInvalidJSON, issues[0].Msg)
	}
	errs := make([]protocol.ValidationFailure, len(issues))
	for i, is := range issues {
		errs[i] = validationFailure(is, data, camelize)
	}
	return protocol.ValidationProblem(errs)
}

func validationFailure(is Issue, data any, camelize bool) protocol.ValidationFailure {
	var path []any
	var missing any
	switch is.Type {
	case "union_tag_not_found":
		path, missing = []any{}, protocol.DiscriminatorField
	case "union_tag_invalid":
		path = []any{protocol.DiscriminatorField}
	default:
		var absent any
		path, absent = protocol.InstancePath(is.Loc, data)
		if is.Type == "missing" {
			missing = absent
		}
	}
	if camelize {
		for i, seg := range path {
			if s, ok := seg.(string); ok {
				path[i] = protocol.Camelize(s)
			}
		}
		if s, ok := missing.(string); ok {
			missing = protocol.Camelize(s)
		}
	}
	return protocol.ValidationFailure{
		Keyword:  keywordFor(is.Type),
		Pointer:  protocol.JSONPointer(path),
		Detail:   is.Msg,
		Property: missing,
	}
}

func detailPayload(format ErrorFormat, typ, detail string) any {
	if format == PydanticErrors {
		return errorDetail{Detail: detail}
	}
	return protocol.NewProblem(typ, detail)
}

func topicErrorPayload(format ErrorFormat, reason, detail string) any {
	if format == PydanticErrors {
		return topicError{Reason: reason, Detail: detail}
	}
	return protocol.NewProblem(protocol.TopicErrorTypes[reason], detail)
}
