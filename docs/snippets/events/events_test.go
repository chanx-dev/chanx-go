package events

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	chanx "github.com/huynguyengl99/chanx-go"
	"github.com/huynguyengl99/chanx-go/chanxtest"
)

func TestFrames(t *testing.T) {
	e := chanx.New(chanx.Config{})
	e.Handle("/ws/notifications/{user}", NotificationConsumer{})
	ann := chanxtest.Connect(t, e, "/ws/notifications/ann")

	mux := http.NewServeMux()
	mux.Handle("POST /orders/{id}/shipped", ShippedHandler(e.Publisher()))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/orders/A17/shipped?user=ann", nil))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code, rec.Body)
	}
	ann.Expect(`{"action": "notification", "payload": {"text": "Order A17 shipped"}}`)

	if err := e.Publisher().BroadcastEvent(context.Background(), Announcement{Payload: "maintenance at 22:00"}, "user.ann"); err != nil {
		t.Fatal(err)
	}
	ann.Expect(`{"action": "announcement", "payload": "maintenance at 22:00"}`)
}
