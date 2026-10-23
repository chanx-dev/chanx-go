// Package protocol holds the chanx wire protocol shared by every implementation:
// envelope fields, framework actions, error reasons, layer event types, topic
// patterns, group names and key casing.
package protocol

// Version is the envelope version stamped on replies and topic frames.
const Version = 1

// Envelope fields travel next to action and payload on the same flat frame and are
// stripped before a message is validated.
const (
	FieldVersion = "version"
	FieldTopic   = "topic"
	FieldRef     = "ref"
	FieldSeq     = "seq"
	FieldAction  = "action"
	FieldPayload = "payload"
)

// EnvelopeFields lists the reserved keys.
var EnvelopeFields = []string{FieldVersion, FieldTopic, FieldRef, FieldSeq}

// IsEnvelopeField reports whether key is reserved for the envelope.
func IsEnvelopeField(key string) bool {
	switch key {
	case FieldVersion, FieldTopic, FieldRef, FieldSeq:
		return true
	}
	return false
}

// Framework actions.
const (
	ActionPing           = "ping"
	ActionPong           = "pong"
	ActionError          = "error"
	ActionAuthentication = "authentication"
	ActionComplete       = "complete"
	ActionEventComplete  = "event_complete"
	ActionGroupComplete  = "group_complete"
	ActionSubscribe      = "subscribe"
	ActionSubscribed     = "subscribed"
	ActionUnsubscribe    = "unsubscribe"
	ActionUnsubscribed   = "unsubscribed"
)

// IsCompleteAction reports the completion actions, never logged.
func IsCompleteAction(action string) bool {
	switch action {
	case ActionComplete, ActionEventComplete, ActionGroupComplete:
		return true
	}
	return false
}

// Topic error reasons, carried in an error frame's payload.
const (
	ReasonUnknownTopic  = "unknown_topic"
	ReasonUnauthorized  = "unauthorized"
	ReasonNotSubscribed = "not_subscribed"
)

// Layer event types: the "type" of the messages chanx puts on a channel layer.
//
//	{"type": "handle_channel_event", "event_data": {...}}
//	{"type": "handle_group_message", "message": {...}, "exclude_current": bool, "from_channel": str}
//	{"type": "handle_topic_event", "topic": str, "seq": int | None, "event_data": {...}}
const (
	EventChannel = "handle_channel_event"
	EventGroup   = "handle_group_message"
	EventTopic   = "handle_topic_event"
)

// DiscriminatorField is the key that selects a message type.
const DiscriminatorField = FieldAction
