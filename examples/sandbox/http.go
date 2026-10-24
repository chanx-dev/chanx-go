package sandbox

import (
	"encoding/json"
	"net/http"

	chanx "github.com/huynguyengl99/chanx-go"
)

// NotifyHandler is the external sender of sandbox_fastapi: POST
// {"message": "..."} pushes a SystemNotify event to the chat room, from plain
// HTTP, through the layer.
func NotifyHandler(e *chanx.Engine) http.Handler {
	chat := e.Publisher().Layer("chat")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Message string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := chat.BroadcastEvent(r.Context(), SystemNotify{Payload: body.Message}, "chat_room"); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})
}
