// Package events holds the examples of the Groups and events guide.
package events

import (
	"encoding/json"
	"net/http"

	chanx "go.chanx.dev/v2"
)

type NotificationPayload struct {
	Text string `json:"text"`
}

// #region event
// OrderShipped is an event: a message between servers. A worker publishes it; each
// connection's event handler decides what, if anything, its client sees.
type OrderShipped struct {
	chanx.BaseMessage `action:"order_shipped"`
	Payload           struct {
		OrderID string `json:"order_id"`
	} `json:"payload"`
}

// Notification is what the client receives.
type Notification struct {
	chanx.BaseMessage `action:"notification"`
	Payload           NotificationPayload `json:"payload"`
}

type NotificationConsumer struct{}

// Groups puts each connection in its user's group.
func (NotificationConsumer) Groups(c *chanx.Conn) []string {
	return []string{"user." + c.Param("user")}
}

// HandleOrderShipped takes an *chanx.EventContext: it handles a layer event, not
// a client frame. Its return value is sent to the client.
func (NotificationConsumer) HandleOrderShipped(ctx *chanx.EventContext, e OrderShipped) (Notification, error) {
	return Notification{Payload: NotificationPayload{Text: "Order " + e.Payload.OrderID + " shipped"}}, nil
}

// #endregion event

// #region passthrough
// Announcement goes to clients as it is: no handler needed.
type Announcement struct {
	chanx.BaseMessage `action:"announcement"`
	Payload           string `json:"payload"`
}

// Passthrough lists events forwarded to the client unchanged.
func (NotificationConsumer) Passthrough() []chanx.Message { return []chanx.Message{Announcement{}} }

// #endregion passthrough

func (NotificationConsumer) HandlePing(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
	return chanx.PongMessage{}, nil
}

// #region publisher
// ShippedHandler is a plain HTTP endpoint: POST /orders/{id}/shipped?user=ann
// pushes to every socket of that user, on any server sharing the layer.
func ShippedHandler(publisher *chanx.Publisher) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var event OrderShipped
		event.Payload.OrderID = r.PathValue("id")
		group := "user." + r.URL.Query().Get("user")
		if err := publisher.BroadcastEvent(r.Context(), event, group); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "sent"})
	})
}

// #endregion publisher
