package codegen

import (
	"go/token"
	"strings"
	"unicode"
)

// goName makes an exported identifier, keeping inner casing: "room_name" and
// "roomName" give RoomName, "PingMessage" stays.
func goName(s string) string {
	var b strings.Builder
	for _, part := range words(s) {
		r := []rune(part)
		b.WriteRune(unicode.ToUpper(r[0]))
		b.WriteString(string(r[1:]))
	}
	return identifier(b.String())
}

// enumName title-cases all-caps values: "PENDING" gives Pending.
func enumName(s string) string {
	var b strings.Builder
	for _, part := range words(s) {
		if strings.ToUpper(part) == part {
			part = strings.ToLower(part)
		}
		r := []rune(part)
		b.WriteRune(unicode.ToUpper(r[0]))
		b.WriteString(string(r[1:]))
	}
	return identifier(b.String())
}

// lowerName makes a parameter name: "room_name" gives roomName.
func lowerName(s string) string {
	n := []rune(goName(s))
	n[0] = unicode.ToLower(n[0])
	name := string(n)
	if token.IsKeyword(name) || name == "ctx" || name == "opts" || name == "baseURL" {
		name += "_"
	}
	return name
}

func words(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

func identifier(s string) string {
	if s == "" {
		return "X"
	}
	if unicode.IsDigit([]rune(s)[0]) {
		return "X" + s
	}
	return s
}
