package protocol

import (
	"strings"
	"unicode"
)

// Camelize, Decamelize and the *Keys variants port pyhumps rule for rule, so
// every chanx implementation cases keys identically. Checked against vectors
// recorded from pyhumps.

// Camelize converts a key to camelCase: "room_name" -> "roomName".
func Camelize(s string) string {
	orig := s
	s = stripSpace(s)
	if isUpper(s) || isNumeric(s) {
		return orig
	}
	r := []rune(s)
	if len(r) != 0 && !isUpper(string(r[:min(2, len(r))])) {
		r[0] = unicode.ToLower(r[0])
	}
	return replaceUnderscores(r)
}

// Decamelize converts a key to snake_case: "roomName" -> "room_name",
// "APIResponse" -> "api_response".
func Decamelize(s string) string {
	orig := s
	s = stripSpace(s)
	if isUpper(s) || isNumeric(s) {
		return orig
	}
	return strings.ToLower(separateWords(fixAbbreviations([]rune(s))))
}

// Depascalize is Decamelize: "ChatMessage" -> "chat_message".
func Depascalize(s string) string { return Decamelize(s) }

// CamelizeKeys camelizes every map key, recursively through maps and lists.
// Values, including strings inside lists, are left alone.
func CamelizeKeys(v any) any { return processKeys(v, Camelize) }

// DecamelizeKeys snake-cases every map key, recursively.
func DecamelizeKeys(v any) any { return processKeys(v, Decamelize) }

func processKeys(v any, fn func(string) string) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[fn(k)] = processKeys(val, fn)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = processKeys(val, fn)
		}
		return out
	}
	return v
}

func stripSpace(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

// isUpper reports at least one cased character and none lowercase.
func isUpper(s string) bool {
	cased := false
	for _, r := range s {
		if unicode.IsLower(r) {
			return false
		}
		if unicode.IsUpper(r) || unicode.IsTitle(r) {
			cased = true
		}
	}
	return cased
}

// isNumeric reports a non-empty string of numeric characters.
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsNumber(r) {
			return false
		}
	}
	return true
}

func isSep(r rune) bool { return r == '-' || r == '_' }

// replaceUnderscores is re.sub(r"(?<=[^\-_])[\-_]+[^\-_]", upper-last-char).
func replaceUnderscores(r []rune) string {
	var b strings.Builder
	for i := 0; i < len(r); {
		if isSep(r[i]) && i > 0 && !isSep(r[i-1]) {
			j := i
			for j < len(r) && isSep(r[j]) {
				j++
			}
			if j < len(r) {
				b.WriteString(strings.ToUpper(string(r[j])))
				i = j + 1
				continue
			}
		}
		b.WriteRune(r[i])
		i++
	}
	return b.String()
}

func isUpperOrDigit(r rune) bool { return (r >= 'A' && r <= 'Z') || unicode.IsDigit(r) }

// fixAbbreviations is re.sub(r"([A-Z\d]+)(?=[A-Z\d]|$)", m.group(0).title()):
// "APIResponse" -> "ApiResponse", so the acronym splits as one word.
func fixAbbreviations(r []rune) []rune {
	out := make([]rune, 0, len(r))
	for i := 0; i < len(r); {
		if !isUpperOrDigit(r[i]) {
			out = append(out, r[i])
			i++
			continue
		}
		j := i
		for j < len(r) && isUpperOrDigit(r[j]) {
			j++
		}
		end := j // the greedy run, backtracked one when not at the end of the string
		if j < len(r) {
			end = j - 1
		}
		if end > i {
			out = append(out, title(r[i:end])...)
			out = append(out, r[end:j]...)
		} else {
			out = append(out, r[i:j]...)
		}
		i = j
	}
	return out
}

// title title-cases a run of capitals and digits: a letter is
// uppercased after an uncased character (start or digit), lowercased otherwise.
func title(r []rune) []rune {
	out := make([]rune, len(r))
	prevCased := false
	for i, c := range r {
		if unicode.IsLetter(c) {
			if prevCased {
				out[i] = unicode.ToLower(c)
			} else {
				out[i] = unicode.ToUpper(c)
			}
			prevCased = true
		} else {
			out[i] = c
			prevCased = false
		}
	}
	return out
}

// separateWords is "_".join(s for s in re.split(r"([\-_]*[A-Z][^A-Z]*[\-_]*)", s) if s).
func separateWords(r []rune) string {
	var pieces []string
	start := 0 // start of the text not yet emitted
	for i := 0; i < len(r); {
		j := i
		for j < len(r) && isSep(r[j]) {
			j++
		}
		if j < len(r) && r[j] >= 'A' && r[j] <= 'Z' {
			pieces = append(pieces, string(r[start:i]))
			k := j + 1
			for k < len(r) && (r[k] < 'A' || r[k] > 'Z') {
				k++
			}
			pieces = append(pieces, string(r[i:k]))
			start, i = k, k
			continue
		}
		i++
	}
	pieces = append(pieces, string(r[start:]))
	var kept []string
	for _, p := range pieces {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, "_")
}
