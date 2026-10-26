// Package otherpkg declares a payload type named like one in the asyncapi tests.
package otherpkg

import chanx "go.chanx.dev/v2"

// RoomPayload clashes by name with asyncapi_test.RoomPayload.
type RoomPayload struct {
	Floor int `json:"floor"`
}

// Other carries the clashing payload.
type Other struct {
	chanx.BaseMessage `action:"other"`
	Payload           RoomPayload `json:"payload"`
}

// Consumer handles Other.
type Consumer struct{}

// HandleOther does nothing.
func (Consumer) HandleOther(ctx *chanx.Context, m Other) error { return nil }
