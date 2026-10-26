package sandbox

import (
	"context"
	"fmt"
	"time"

	chanx "go.chanx.dev/v2"
)

// ping is shared by every consumer below.
type ping struct{}

func (ping) HandlePing(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
	return chanx.PongMessage{}, nil
}

var pingOp = chanx.Operation{Summary: "Handle ping requests", Description: "Simple ping-pong for connectivity testing"}

// Mixins (apps/mixins.py): embed them and their handlers come along.

type ExtraWsHandlerMixin struct{}

func (ExtraWsHandlerMixin) HandleExtraMessage(ctx *chanx.Context, m ExtraRequestMessage) (ExtraResponseMessage, error) {
	return ExtraResponseMessage{Payload: m.Payload + " any extra thing"}, nil
}

type ExtraEventHandlerMixin struct{}

func (ExtraEventHandlerMixin) HandleExtraEvent(ctx *chanx.EventContext, e ExtraEventMessage) (ExtraResponseMessage, error) {
	return ExtraResponseMessage{Payload: e.Payload + " any extra thing"}, nil
}

type ExtraPassthroughMixin struct{}

func (ExtraPassthroughMixin) Passthrough() []chanx.Message {
	return []chanx.Message{ExtraPassthroughMessage{}}
}

// SystemMessageConsumer: direct WebSocket, no groups.
type SystemMessageConsumer struct{ ping }

func (SystemMessageConsumer) HandleSystem(ctx *chanx.Context, m UserMessage) (SystemEchoMessage, error) {
	return SystemEchoMessage{Payload: MessagePayload{Message: "🔧 System Echo: " + m.Payload.Message}}, nil
}

func (SystemMessageConsumer) OnConnect(ctx *chanx.Context) error {
	return ctx.Push(SystemEchoMessage{Payload: MessagePayload{Message: "🔧 System: Connection established!"}})
}

func (SystemMessageConsumer) Operations() map[string]chanx.Operation {
	return map[string]chanx.Operation{
		"HandlePing":   pingOp,
		"HandleSystem": {Summary: "Handle message user send to system", Description: "Echo system messages back directly without using channel layers"},
	}
}

// RoomChatConsumer: one group per room from the URL.
type RoomChatConsumer struct{ ping }

func roomGroup(conn *chanx.Conn) string { return "room_" + conn.Param("room_name") }

func (RoomChatConsumer) Groups(conn *chanx.Conn) []string { return []string{roomGroup(conn)} }

func (RoomChatConsumer) HandleRoomChat(ctx *chanx.Context, m RoomChatMessage) error {
	room := ctx.Conn.Param("room_name")
	return ctx.Broadcast(RoomNotificationMessage{Payload: RoomMessagePayload{
		Message: "💬 " + m.Payload.Message, RoomName: &room, Status: StatusPending,
	}})
}

func (RoomChatConsumer) OnConnect(ctx *chanx.Context) error {
	room := ctx.Conn.Param("room_name")
	return ctx.Broadcast(RoomNotificationMessage{Payload: RoomMessagePayload{
		Message: fmt.Sprintf("🚪 Someone joined room '%s'", room), RoomName: &room, Status: StatusPending,
	}}, chanx.ExcludeSelf())
}

func (RoomChatConsumer) OnDisconnect(ctx *chanx.Context) {
	room := ctx.Conn.Param("room_name")
	_ = ctx.Broadcast(RoomNotificationMessage{Payload: RoomMessagePayload{
		Message: fmt.Sprintf("👋 Someone left room '%s'", room), RoomName: &room, Status: StatusPending,
	}}, chanx.ExcludeSelf())
}

func (RoomChatConsumer) Operations() map[string]chanx.Operation {
	return map[string]chanx.Operation{
		"HandlePing": pingOp,
		"HandleRoomChat": {Summary: "Handle room chat messages", Description: "Process room chat messages and broadcast to room members",
			Replies: []chanx.Message{RoomNotificationMessage{}}},
	}
}

// ChatConsumer: the showcase chat, with mixins.
type ChatConsumer struct {
	ExtraWsHandlerMixin
	ExtraPassthroughMixin
	ping
}

func (ChatConsumer) Groups(*chanx.Conn) []string { return []string{"chat_room"} }

// Passthrough is merged with ExtraPassthroughMixin's.
func (ChatConsumer) Passthrough() []chanx.Message {
	return []chanx.Message{UserJoinedNotification{}, UserLeftNotification{}}
}

func (ChatConsumer) HandleChat(ctx *chanx.Context, m ChatMessage) error {
	return ctx.Broadcast(ChatNotificationMessage{Payload: ChatPayload{Message: "💬 " + m.Payload.Message}})
}

func (ChatConsumer) OnConnect(ctx *chanx.Context) error {
	return ctx.Broadcast(ChatNotificationMessage{Payload: ChatPayload{Message: "📢 Someone joined the chat"}})
}

func (ChatConsumer) OnDisconnect(ctx *chanx.Context) {
	_ = ctx.Broadcast(ChatNotificationMessage{Payload: ChatPayload{Message: "❌ Someone left the chat."}})
}

func (ChatConsumer) SystemNotifyChat(ctx *chanx.EventContext, e SystemNotify) (ChatNotificationMessage, error) {
	return ChatNotificationMessage{Payload: ChatPayload{Message: e.Payload}}, nil
}

func (ChatConsumer) Operations() map[string]chanx.Operation {
	return map[string]chanx.Operation{
		"HandlePing":         pingOp,
		"HandleExtraMessage": {Summary: "Handle extra request", Description: "Simple extra message"},
		"HandleChat": {Summary: "Handle chat messages", Description: "Process chat messages and broadcast to room",
			Replies: []chanx.Message{ChatNotificationMessage{}}},
	}
}

// ReliableChatConsumer broadcasts on the "queue" layer.
type ReliableChatConsumer struct{ ping }

func (ReliableChatConsumer) Groups(*chanx.Conn) []string { return []string{"reliable_chat"} }

func (ReliableChatConsumer) HandleReliableChat(ctx *chanx.Context, m ReliableChatMessage) error {
	return ctx.Broadcast(ReliableChatNotificationMessage{Payload: ReliableChatPayload{Message: "📨 " + m.Payload.Message}},
		chanx.ToGroups("reliable_chat"))
}

func (ReliableChatConsumer) OnConnect(ctx *chanx.Context) error {
	return ctx.Broadcast(ReliableChatNotificationMessage{Payload: ReliableChatPayload{Message: "🔒 Reliable chat connection established!"}},
		chanx.ToGroups("reliable_chat"))
}

func (ReliableChatConsumer) OnDisconnect(ctx *chanx.Context) {
	_ = ctx.Broadcast(ReliableChatNotificationMessage{Payload: ReliableChatPayload{Message: "🚪 Left reliable chat!"}},
		chanx.ToGroups("reliable_chat"))
}

func (ReliableChatConsumer) SystemNotifyChat(ctx *chanx.EventContext, e SystemNotify) (ChatNotificationMessage, error) {
	return ChatNotificationMessage{Payload: ChatPayload{Message: e.Payload}}, nil
}

func (ReliableChatConsumer) Operations() map[string]chanx.Operation {
	return map[string]chanx.Operation{
		"HandlePing": pingOp,
		"HandleReliableChat": {Summary: "Handle reliable chat messages", Description: "Process reliable chat messages with guaranteed delivery",
			Replies: []chanx.Message{ReliableChatNotificationMessage{}}},
	}
}

// NotificationConsumer: real-time notifications.
type NotificationConsumer struct{ ping }

func (NotificationConsumer) Groups(*chanx.Conn) []string { return []string{"notifications"} }

func (NotificationConsumer) HandleNotification(ctx *chanx.Context, m NotificationMessage) error {
	return ctx.Broadcast(NotificationBroadcastMessage{Payload: NotificationPayload{Type: "user", Message: "🔔 Notification: " + m.Payload.Message}})
}

func (NotificationConsumer) OnConnect(ctx *chanx.Context) error {
	return ctx.Broadcast(NotificationBroadcastMessage{Payload: NotificationPayload{Type: "system", Message: "🔔 Connected to notifications!"}})
}

func (NotificationConsumer) SystemNotifyNotification(ctx *chanx.EventContext, e SystemNotify) (NotificationBroadcastMessage, error) {
	return NotificationBroadcastMessage{Payload: NotificationPayload{Type: "system", Message: e.Payload}}, nil
}

func (NotificationConsumer) SystemNotifyPeriodicNotification(ctx *chanx.EventContext, e SystemPeriodicNotify) (NotificationBroadcastMessage, error) {
	return NotificationBroadcastMessage{Payload: NotificationPayload{Type: "periodic", Message: e.Payload}}, nil
}

func (NotificationConsumer) Operations() map[string]chanx.Operation {
	return map[string]chanx.Operation{
		"HandlePing": pingOp,
		"HandleNotification": {Summary: "Handle notification messages", Description: "Process notification messages and broadcast to all clients",
			Replies: []chanx.Message{NotificationBroadcastMessage{}}},
	}
}

// AnalyticsConsumer: analytics events.
type AnalyticsConsumer struct{ ping }

func (AnalyticsConsumer) Groups(*chanx.Conn) []string { return []string{"analytics"} }

func (AnalyticsConsumer) HandleAnalytics(ctx *chanx.Context, m AnalyticsMessage) error {
	return ctx.Broadcast(AnalyticsNotificationMessage{Payload: AnalyticsPayload{Event: "📊 Analytics: " + m.Payload.Event, Data: m.Payload.Data}})
}

func (AnalyticsConsumer) SystemNotifyAnalytic(ctx *chanx.EventContext, e SystemNotify) (AnalyticsNotificationMessage, error) {
	return AnalyticsNotificationMessage{Payload: AnalyticsPayload{Event: e.Payload}}, nil
}

func (AnalyticsConsumer) Operations() map[string]chanx.Operation {
	return map[string]chanx.Operation{
		"HandlePing": pingOp,
		"HandleAnalytics": {Summary: "Handle analytics events", Description: "Process analytics events with reliable delivery",
			Replies: []chanx.Message{AnalyticsNotificationMessage{}}},
	}
}

// BackgroundJobConsumer queues jobs a worker answers with a JobResult event sent
// to this connection's channel; a goroutine stands in for the worker.
type BackgroundJobConsumer struct {
	ExtraEventHandlerMixin
	ping
	// Queue runs a job and returns its result; nil uses a built-in stand-in.
	Queue func(ctx context.Context, jobType, content string) (any, error)
}

func (BackgroundJobConsumer) Groups(*chanx.Conn) []string { return []string{"background_jobs"} }

func (b BackgroundJobConsumer) HandleJob(ctx *chanx.Context, m JobMessage) error {
	jobType, content := m.Payload.Type, m.Payload.Content
	if err := ctx.Send(JobStatusMessage{Payload: map[string]any{
		"status": "queuing", "message": fmt.Sprintf("⏳ Queuing %s job: %s", jobType, content),
	}}); err != nil {
		return err
	}
	jobID := "job-" + time.Now().Format("150405.000")
	pub, channel := ctx.Publisher(), ctx.Conn.ChannelName
	run := b.Queue
	if run == nil {
		run = processJob
	}
	go func() {
		result, err := run(context.Background(), jobType, content)
		if err != nil {
			result = "❌ " + err.Error()
		}
		_ = pub.SendEvent(context.Background(), channel, JobResult{Payload: result})
	}()
	return ctx.Send(JobStatusMessage{Payload: map[string]any{
		"status": "queued", "job_id": jobID,
		"message": fmt.Sprintf("📋 Job %s queued successfully! Worker will process it shortly...", jobID),
	}})
}

func processJob(_ context.Context, jobType, content string) (any, error) {
	switch jobType {
	case "translate":
		return "🌐 Translated: " + content, nil
	case "analyze":
		return fmt.Sprintf("📊 Analysis: %d characters", len([]rune(content))), nil
	case "generate":
		return "🤖 Generated: " + content + "...", nil
	}
	return "✅ Processed: " + content, nil
}

func (BackgroundJobConsumer) OnConnect(ctx *chanx.Context) error {
	return ctx.Push(JobStatusMessage{Payload: map[string]any{"status": "connected", "message": "🔄 Background Job Processor: Connected!"}})
}

func (BackgroundJobConsumer) HandleJobResult(ctx *chanx.EventContext, e JobResult) (JobStatusMessage, error) {
	return JobStatusMessage{Payload: map[string]any{"status": "result", "message": e.Payload}}, nil
}

func (BackgroundJobConsumer) Operations() map[string]chanx.Operation {
	return map[string]chanx.Operation{
		"HandlePing":       pingOp,
		"HandleJob":        {Summary: "Handle job processing requests", Description: "Process job requests by queuing them with ARQ", Replies: []chanx.Message{JobStatusMessage{}}},
		"HandleExtraEvent": {Summary: "Handle extra event", Description: "Simple extra event message"},
	}
}

// Topics (apps/topics/consumer.py).

type RoomTopic struct {
	chanx.BaseTopic `pattern:"room:{room_name}" layer:"chat" description:"A chat room, addressed as room:<room_name>."`
}

func (RoomTopic) HandlePost(ctx *chanx.Context, m PostMessage) error {
	return ctx.BroadcastTopic(RoomEvent{Payload: m.Payload})
}

func (RoomTopic) HandleEcho(ctx *chanx.Context, m EchoMessage) (PostedMessage, error) {
	return PostedMessage{Payload: m.Payload}, nil
}

func (RoomTopic) Passthrough() []chanx.Message { return []chanx.Message{RoomEvent{}} }

func (RoomTopic) Operations() map[string]chanx.Operation {
	return map[string]chanx.Operation{
		"HandlePost": {Summary: "Post to a room"},
		"HandleEcho": {Summary: "Acknowledge without broadcasting"},
	}
}

type PresenceTopic struct {
	chanx.BaseTopic `pattern:"presence:{user}" layer:"chat" description:"Presence for one user, addressed as presence:<user>."`
}

func (PresenceTopic) Passthrough() []chanx.Message { return []chanx.Message{PresenceChanged{}} }

type TopicHubConsumer struct{ ping }

func (TopicHubConsumer) Topics() []chanx.TopicHandler {
	return []chanx.TopicHandler{&RoomTopic{}, &PresenceTopic{}}
}

func (TopicHubConsumer) Operations() map[string]chanx.Operation {
	return map[string]chanx.Operation{"HandlePing": {Summary: "Handle ping requests"}}
}
