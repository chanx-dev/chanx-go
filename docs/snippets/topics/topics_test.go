package topics

import (
	"context"
	"testing"

	chanx "go.chanx.dev"
	"go.chanx.dev/chanxtest"
)

func TestFrames(t *testing.T) {
	e := chanx.New(chanx.Config{})
	Mount(e)
	c := chanxtest.Connect(t, e, "/ws/market")

	c.SendText(`{"action": "subscribe", "topic": "ticker:ACME", "ref": "s1"}`)
	c.Expect(`{"action": "subscribed", "payload": null, "version": 1, "topic": "ticker:ACME", "ref": "s1"}`)
	c.Expect(`{"action": "price_update", "payload": {"symbol": "ACME", "price": 100.0}, "version": 1, "topic": "ticker:ACME"}`)
	c.SendText(`{"action": "quote", "topic": "ticker:ACME", "ref": "q1", "payload": {"price": 101.5}}`)
	c.Expect(`{"action": "price_update", "payload": {"symbol": "ACME", "price": 101.5}, "version": 1, "topic": "ticker:ACME", "ref": "q1"}`)
	c.SendText(`{"action": "subscribe", "topic": "ticker:SECRET"}`)
	c.Expect(`{"action": "error", "payload": {"reason": "unauthorized", "detail": "Not allowed to subscribe to 'ticker:SECRET'"}, "version": 1, "topic": "ticker:SECRET"}`)
	c.SendText(`{"action": "quote", "topic": "ticker:OTHER", "payload": {"price": 1}}`)
	c.Expect(`{"action": "error", "payload": {"reason": "not_subscribed", "detail": "Not subscribed to 'ticker:OTHER'"}, "version": 1, "topic": "ticker:OTHER"}`)
	c.SendText(`{"action": "subscribe", "topic": "weather:paris"}`)
	c.Expect(`{"action": "error", "payload": {"reason": "unknown_topic", "detail": "Unknown topic 'weather:paris'"}, "version": 1, "topic": "weather:paris"}`)
	if err := PublishPrice(context.Background(), e.Publisher(), "ACME", 102.25, 7); err != nil {
		t.Fatal(err)
	}
	c.Expect(`{"action": "price_update", "payload": {"symbol": "ACME", "price": 102.25}, "version": 1, "topic": "ticker:ACME", "seq": 7}`)
	c.SendText(`{"action": "unsubscribe", "topic": "ticker:ACME", "ref": "u1"}`)
	c.Expect(`{"action": "unsubscribed", "payload": null, "version": 1, "topic": "ticker:ACME", "ref": "u1"}`)

	d := chanxtest.Connect(t, e, "/ws/ticker/ACME")
	d.Expect(`{"action": "subscribed", "payload": null, "version": 1, "topic": "ticker:ACME"}`)
	d.Expect(`{"action": "price_update", "payload": {"symbol": "ACME", "price": 100.0}, "version": 1, "topic": "ticker:ACME"}`)
	d.SendText(`{"action": "quote", "payload": {"price": 99}}`)
	d.Expect(`{"action": "price_update", "payload": {"symbol": "ACME", "price": 99.0}, "version": 1, "topic": "ticker:ACME"}`)
}
