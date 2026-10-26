package chanx

import (
	"errors"
	"fmt"
	"reflect"
	"sync"

	"go.chanx.dev/internal/schema"
	"go.chanx.dev/protocol"
)

// BaseMessage marks a struct as a chanx message. Embed it with the action in
// its tag, plus an optional Payload field:
//
//	type ChatMessage struct {
//		chanx.BaseMessage `action:"chat" description:"Send a chat message"`
//		Payload ChatPayload `json:"payload"`
//	}
//
// It is zero-size and adds nothing to the JSON. Tag keys: action (required),
// description (message and operation docs) and title (overrides the AsyncAPI
// name, the Go type name by default).
type BaseMessage struct{}

func (BaseMessage) chanxMessage() {}

// Message is any struct embedding BaseMessage.
type Message interface{ chanxMessage() }

// PingMessage asks the server for a PongMessage.
type PingMessage struct {
	BaseMessage `action:"ping" description:"Simple ping message to check WebSocket connection status."`
}

// PongMessage answers a PingMessage.
type PongMessage struct {
	BaseMessage `action:"pong" description:"Simple pong message response to ping requests."`
}

// ErrorMessage communicates issues to the client: validation issues, or a
// detail (and for topics a reason).
type ErrorMessage struct {
	BaseMessage `action:"error" description:"Error message for communicating issues to the client."`
	Payload     any `json:"payload,omitempty"`
}

// AuthenticationPayload is the result of authenticating a connection.
type AuthenticationPayload struct {
	StatusCode int    `json:"status_code"`
	StatusText string `json:"status_text"`
	Data       any    `json:"data" default:"null"`
}

// AuthenticationMessage tells the client how authentication went.
type AuthenticationMessage struct {
	BaseMessage `action:"authentication" description:"Authentication result message sent to the client."`
	Payload     AuthenticationPayload `json:"payload"`
}

// CompleteMessage is sent after a frame was fully processed (SendCompletion).
type CompleteMessage struct {
	BaseMessage `action:"complete" description:"Confirmation message indicating processing is complete."`
}

// GroupCompleteMessage follows a forwarded group message (SendCompletion).
type GroupCompleteMessage struct {
	BaseMessage `action:"group_complete" description:"Confirmation message indicating group message processing is complete."`
}

// EventCompleteMessage follows a processed channel event (SendCompletion).
type EventCompleteMessage struct {
	BaseMessage `action:"event_complete" description:"Confirmation message indicating event processing is complete."`
}

// SubscribeMessage asks to subscribe to the topic named by the frame.
type SubscribeMessage struct {
	BaseMessage `action:"subscribe" description:"Ask to subscribe to the topic named by the frame."`
}

// SubscribedMessage confirms a subscription.
type SubscribedMessage struct {
	BaseMessage `action:"subscribed" description:"Confirm a subscription."`
}

// UnsubscribeMessage asks to unsubscribe from the topic named by the frame.
type UnsubscribeMessage struct {
	BaseMessage `action:"unsubscribe" description:"Ask to unsubscribe from the topic named by the frame."`
}

// UnsubscribedMessage confirms an unsubscription.
type UnsubscribedMessage struct {
	BaseMessage `action:"unsubscribed" description:"Confirm an unsubscription."`
}

// errorDetail and topicError fix the key order of error payloads.
type errorDetail struct {
	Detail string `json:"detail"`
}

type topicError struct {
	Reason string `json:"reason"`
	Detail string `json:"detail"`
}

// MessageInfo describes a message type: what its tag declares and its payload.
type MessageInfo struct {
	Type        reflect.Type
	Action      string
	Title       string // AsyncAPI name: the title tag or the type name
	Description string
	// PayloadType is nil for a message without a Payload field (payload null).
	PayloadType reflect.Type
	// PayloadOptional is set by omitempty, omitzero or a default tag; otherwise a
	// frame without a payload is invalid.
	PayloadOptional bool
	PayloadDefault  string
	HasDefault      bool
	payloadIndex    []int

	once      sync.Once
	validator *schema.Validator
	vErr      error
}

var (
	baseMessageType = reflect.TypeFor[BaseMessage]()
	messageType     = reflect.TypeFor[Message]()
	messageInfos    sync.Map // reflect.Type -> *MessageInfo
)

// Info returns what chanx reads from a message type.
func Info(m Message) (*MessageInfo, error) { return infoOf(reflect.TypeOf(m)) }

// ActionOf returns a message's action.
func ActionOf(m Message) string {
	info, err := Info(m)
	if err != nil {
		return ""
	}
	return info.Action
}

func infoOf(t reflect.Type) (*MessageInfo, error) {
	if t == nil {
		return nil, errors.New("chanx: nil message")
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if v, ok := messageInfos.Load(t); ok {
		return v.(*MessageInfo), nil
	}
	if t.Kind() != reflect.Struct || !t.Implements(messageType) {
		return nil, fmt.Errorf("chanx: %s is not a message: embed chanx.BaseMessage", t)
	}
	info := &MessageInfo{Type: t, Title: t.Name()}
	base, ok := t.FieldByName("BaseMessage")
	if !ok || base.Type != baseMessageType || len(base.Index) != 1 {
		return nil, fmt.Errorf("chanx: %s must embed chanx.BaseMessage directly", t)
	}
	info.Action = base.Tag.Get("action")
	info.Description = base.Tag.Get("description")
	if title := base.Tag.Get("title"); title != "" {
		info.Title = title
	}
	if info.Action == "" {
		return nil, fmt.Errorf("chanx: %s embeds chanx.BaseMessage without an `action` tag", t)
	}
	for _, f := range schema.Fields(t) {
		if f.Name == protocol.FieldPayload {
			info.PayloadType = f.Type
			info.PayloadOptional = f.Optional
			info.PayloadDefault, info.HasDefault = f.Default, f.HasDef
			info.payloadIndex = f.Index
		}
	}
	if info.PayloadType == nil {
		if _, ok := t.FieldByName("Payload"); ok {
			return nil, fmt.Errorf("chanx: %s has a Payload field not named \"payload\" in JSON", t)
		}
	}
	v, _ := messageInfos.LoadOrStore(t, info)
	return v.(*MessageInfo), nil
}

// payloadValidator compiles the payload schema on first use.
func (m *MessageInfo) payloadValidator() (*schema.Validator, error) {
	m.once.Do(func() {
		if m.PayloadType != nil {
			m.validator, m.vErr = schema.NewValidator(m.PayloadType)
			if m.vErr != nil {
				m.vErr = fmt.Errorf("chanx: payload schema of %s: %w", m.Type, m.vErr)
			}
		}
	})
	return m.validator, m.vErr
}

// payloadOf returns the message's payload value, nil for none.
func payloadOf(m Message) (*MessageInfo, any, error) {
	v := reflect.ValueOf(m)
	if !v.IsValid() {
		return nil, nil, errors.New("chanx: nil message")
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, nil, errors.New("chanx: nil message")
		}
		v = v.Elem()
	}
	info, err := infoOf(v.Type())
	if err != nil {
		return nil, nil, err
	}
	if info.PayloadType == nil {
		return info, nil, nil
	}
	return info, v.FieldByIndex(info.payloadIndex).Interface(), nil
}

// Check builds a consumer's or topic's definition without mounting it, for a
// one-line unit test of handler signatures and duplicate actions.
func Check(consumer any) error {
	_, err := buildAny(consumer)
	return err
}

// ApplyDefaults fills a struct's `default` tags, for values decoded outside chanx
// (an HTTP body, a queue job) that should match what a frame would get. Call it
// before decoding.
func ApplyDefaults(ptr any) { schema.ApplyDefaults(reflect.ValueOf(ptr)) }
