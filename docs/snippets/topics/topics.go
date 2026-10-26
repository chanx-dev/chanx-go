// Package topics holds the examples of the Topics guide.
package topics

import (
	"context"

	chanx "go.chanx.dev/v2"
)

type PricePayload struct {
	Symbol string  `json:"symbol"`
	Price  float64 `json:"price"`
}

// PriceUpdate is what subscribers of a ticker receive.
type PriceUpdate struct {
	chanx.BaseMessage `action:"price_update"`
	Payload           PricePayload `json:"payload"`
}

type QuotePayload struct {
	Price float64 `json:"price"`
}

// QuoteMessage asks for a quote inside a ticker topic.
type QuoteMessage struct {
	chanx.BaseMessage `action:"quote"`
	Payload           QuotePayload `json:"payload"`
}

// #region topic
// TickerTopic is one stream per symbol: clients subscribe to "ticker:ACME".
type TickerTopic struct {
	chanx.BaseTopic `pattern:"ticker:{symbol}" description:"Live prices for one symbol"`
}

// Authorize decides who may subscribe; false answers an "unauthorized" error.
func (TickerTopic) Authorize(ctx *chanx.Context, p chanx.Params) (bool, error) {
	return p["symbol"] != "SECRET", nil
}

// OnSubscribe runs once per subscription: push the current state first.
func (TickerTopic) OnSubscribe(ctx *chanx.Context) error {
	return ctx.Push(PriceUpdate{Payload: PricePayload{Symbol: ctx.Params["symbol"], Price: 100}})
}

// HandleQuote handles a frame sent to the topic, and replies to the sender.
func (TickerTopic) HandleQuote(ctx *chanx.Context, m QuoteMessage) (PriceUpdate, error) {
	return PriceUpdate{Payload: PricePayload{Symbol: ctx.Params["symbol"], Price: m.Payload.Price}}, nil
}

// Passthrough forwards PriceUpdate events published to the topic to its
// subscribers unchanged. Without it, or an event handler, they are dropped.
func (TickerTopic) Passthrough() []chanx.Message { return []chanx.Message{PriceUpdate{}} }

// #endregion topic

// #region hub
// MarketConsumer hosts the ticker topic: one socket, any number of symbols.
type MarketConsumer struct{}

func (MarketConsumer) Topics() []chanx.TopicHandler { return []chanx.TopicHandler{TickerTopic{}} }

func Mount(engine *chanx.Engine) {
	engine.Handle("/ws/market", MarketConsumer{})
	engine.Handle("/ws/ticker/{symbol}", chanx.TopicConsumer(TickerTopic{})) // one symbol per socket
}

// #endregion hub

// #region publish
// PublishPrice pushes a price to every subscriber of the symbol, from a worker or
// any process sharing the layer. seq lets clients detect a gap.
func PublishPrice(ctx context.Context, publisher *chanx.Publisher, symbol string, price float64, seq int64) error {
	topic, err := chanx.FormatTopic[TickerTopic](chanx.Params{"symbol": symbol})
	if err != nil {
		return err
	}
	update := PriceUpdate{Payload: PricePayload{Symbol: symbol, Price: price}}
	return chanx.BroadcastTopic[TickerTopic](ctx, publisher, topic, update, chanx.Seq(seq))
}

// #endregion publish
