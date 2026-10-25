// Package messages holds the message examples of the Messages guide.
package messages

import (
	"errors"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"

	chanx "github.com/huynguyengl99/chanx-go"
)

// #region basic
type PostPayload struct {
	Body   string   `json:"body"`                       // required
	Author string   `json:"author" default:"anonymous"` // filled when absent
	Tags   []string `json:"tags,omitempty"`             // optional
	Reply  *string  `json:"reply_to,omitempty"`         // optional and nullable
}

type PostMessage struct {
	chanx.BaseMessage `action:"post" description:"Post something to the room."`
	Payload           PostPayload `json:"payload"`
}

// TypingMessage has no payload: clients send {"action": "typing"}.
type TypingMessage struct {
	chanx.BaseMessage `action:"typing"`
}

// #endregion basic

// #region enum
// Mood is a string enum, Pydantic's StrEnum.
type Mood string

func (Mood) JSONSchema(s *jsonschema.Schema) { s.Enum = []any{"happy", "sad"} }

type MoodPayload struct {
	Mood Mood `json:"mood"`
}

type MoodMessage struct {
	chanx.BaseMessage `action:"mood"`
	Payload           MoodPayload `json:"payload"`
}

// #endregion enum

// #region constraints
type SignupPayload struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// JSONSchema adds constraints Go tags cannot say, as Field(min_length=3) does.
// They validate frames and appear in the AsyncAPI document.
func (SignupPayload) JSONSchema(s *jsonschema.Schema) {
	min := 3
	s.Properties["username"].MinLength = &min
}

// Validate runs after the schema checks, as a Pydantic validator does.
func (p SignupPayload) Validate() error {
	if strings.Contains(p.Password, p.Username) {
		return errors.New("password must not contain the username")
	}
	return nil
}

type SignupMessage struct {
	chanx.BaseMessage `action:"signup"`
	Payload           SignupPayload `json:"payload"`
}

// #endregion constraints

// Room accepts every message above and echoes posts back.
type Room struct{}

func (Room) HandlePost(ctx *chanx.Context, m PostMessage) (PostMessage, error) { return m, nil }
func (Room) HandleTyping(ctx *chanx.Context, _ TypingMessage) error            { return nil }
func (Room) HandleMood(ctx *chanx.Context, m MoodMessage) error                { return nil }
func (Room) HandleSignup(ctx *chanx.Context, m SignupMessage) error            { return nil }
