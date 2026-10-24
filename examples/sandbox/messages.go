package sandbox

import (
	"github.com/google/jsonschema-go/jsonschema"

	chanx "github.com/huynguyengl99/chanx-go"
)

// Mixin messages (sandbox_fastapi/apps/mixins.py).

type ExtraRequestMessage struct {
	chanx.BaseMessage `action:"extra_request" description:"Extra request message for ws handler mixin."`
	Payload           string `json:"payload"`
}

type ExtraEventMessage struct {
	chanx.BaseMessage `action:"extra_event" description:"Extra event message for event handler mixin."`
	Payload           string `json:"payload"`
}

type ExtraResponseMessage struct {
	chanx.BaseMessage `action:"extra_response" description:"Extra response message returned by handler mixins."`
	Payload           string `json:"payload"`
}

type ExtraPassthroughMessage struct {
	chanx.BaseMessage `action:"extra_passthrough" description:"Event forwarded straight to the client by a mixin's passthrough_events."`
	Payload           string `json:"payload"`
}

// System chat (apps/system_chat/messages.py).

type MessagePayload struct {
	Message string `json:"message"`
}

type UserMessage struct {
	chanx.BaseMessage `action:"user_message" description:"System message for direct communication."`
	Payload           MessagePayload `json:"payload"`
}

type SystemEchoMessage struct {
	chanx.BaseMessage `action:"system_echo" description:"System echo response message."`
	Payload           MessagePayload `json:"payload"`
}

// Room chat (apps/room_chat/messages.py).

// MessageStatus is a string enum.
type MessageStatus string

const (
	StatusPending   MessageStatus = "PENDING"
	StatusDelivered MessageStatus = "DELIVERED"
	StatusFailed    MessageStatus = "FAILED"
)

func (MessageStatus) JSONSchema(s *jsonschema.Schema) {
	s.Enum = []any{"PENDING", "DELIVERED", "FAILED"}
}

type RoomMessagePayload struct {
	Message  string        `json:"message"`
	RoomName *string       `json:"room_name,omitempty"`
	Status   MessageStatus `json:"status" default:"PENDING"`
}

type RoomChatMessage struct {
	chanx.BaseMessage `action:"room_chat" description:"Room chat message."`
	Payload           RoomMessagePayload `json:"payload"`
}

type RoomNotificationMessage struct {
	chanx.BaseMessage `action:"room_notification" description:"Room notification message."`
	Payload           RoomMessagePayload `json:"payload"`
}

// Showcase (apps/showcase/messages.py).

type ChatPayload struct {
	Message string `json:"message"`
}

type ChatMessage struct {
	chanx.BaseMessage `action:"chat" description:"Chat message for the basic chat consumer."`
	Payload           ChatPayload `json:"payload"`
}

type ChatNotificationMessage struct {
	chanx.BaseMessage `action:"chat_notification" description:"Chat notification message."`
	Payload           ChatPayload `json:"payload"`
}

type ReliableChatPayload struct {
	Message string `json:"message"`
}

type ReliableChatMessage struct {
	chanx.BaseMessage `action:"reliable_chat" description:"Reliable chat message."`
	Payload           ReliableChatPayload `json:"payload"`
}

type ReliableChatNotificationMessage struct {
	chanx.BaseMessage `action:"reliable_chat_notification" description:"Reliable chat notification message."`
	Payload           ReliableChatPayload `json:"payload"`
}

type NotificationPayload struct {
	Type    string `json:"type" default:"user"`
	Message string `json:"message"`
}

type NotificationMessage struct {
	chanx.BaseMessage `action:"notification" description:"Notification message."`
	Payload           NotificationPayload `json:"payload"`
}

type NotificationBroadcastMessage struct {
	chanx.BaseMessage `action:"notification_broadcast" description:"Notification broadcast message."`
	Payload           NotificationPayload `json:"payload"`
}

type AnalyticsPayload struct {
	Event string `json:"event"`
	Data  any    `json:"data,omitempty"`
}

type AnalyticsMessage struct {
	chanx.BaseMessage `action:"analytics" description:"Analytics message."`
	Payload           AnalyticsPayload `json:"payload"`
}

type AnalyticsNotificationMessage struct {
	chanx.BaseMessage `action:"analytics_notification" description:"Analytics notification message."`
	Payload           AnalyticsPayload `json:"payload"`
}

type SystemNotify struct {
	chanx.BaseMessage `action:"system_notify"`
	Payload           string `json:"payload"`
}

type SystemPeriodicNotify struct {
	chanx.BaseMessage `action:"system_periodic_notify"`
	Payload           string `json:"payload"`
}

type UserJoinedNotification struct {
	chanx.BaseMessage `action:"user_joined_notification" description:"Notification when a user joins the chat."`
	Payload           ChatPayload `json:"payload"`
}

type UserLeftNotification struct {
	chanx.BaseMessage `action:"user_left_notification" description:"Notification when a user leaves the chat."`
	Payload           ChatPayload `json:"payload"`
}

// Background jobs (apps/background_jobs/messages.py).

type JobPayload struct {
	Type    string `json:"type" default:"default"`
	Content string `json:"content"`
}

type JobMessage struct {
	chanx.BaseMessage `action:"job" description:"Background job message."`
	Payload           JobPayload `json:"payload"`
}

type JobStatusMessage struct {
	chanx.BaseMessage `action:"job_status" description:"Job status message."`
	Payload           map[string]any `json:"payload"`
}

type JobResult struct {
	chanx.BaseMessage `action:"job_result" description:"Background job message."`
	Payload           any `json:"payload"`
}

// Topics (apps/topics/messages.py).

type PostPayload struct {
	Body   string `json:"body"`
	Author string `json:"author" default:"anonymous"`
}

type PostMessage struct {
	chanx.BaseMessage `action:"post" description:"Post something to a room."`
	Payload           PostPayload `json:"payload"`
}

type EchoMessage struct {
	chanx.BaseMessage `action:"echo" description:"Ask for an acknowledgement without broadcasting."`
	Payload           PostPayload `json:"payload"`
}

type PostedMessage struct {
	chanx.BaseMessage `action:"posted" description:"Confirm a post."`
	Payload           PostPayload `json:"payload"`
}

type RoomEvent struct {
	chanx.BaseMessage `action:"room_event" description:"Something happened in a room."`
	Payload           PostPayload `json:"payload"`
}

type PresencePayload struct {
	User   string `json:"user"`
	Online bool   `json:"online"`
}

type PresenceChanged struct {
	chanx.BaseMessage `action:"presence_changed" description:"A user's presence changed."`
	Payload           PresencePayload `json:"payload"`
}
