// Command tutorial is the chat server built in the tutorial.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"go.chanx.dev/layers/redis/v2"
	"go.chanx.dev/layers/v2"
	chanx "go.chanx.dev/v2"
	"go.chanx.dev/v2/asyncapi"
	"go.chanx.dev/v2/auth"
	chanxhttp "go.chanx.dev/v2/nethttp"
)

// #region messages
type SendPayload struct {
	Text string `json:"text"`
}

// SendMessage posts to the room the connection is in.
type SendMessage struct {
	chanx.BaseMessage `action:"send" description:"Post a message to the room."`
	Payload           SendPayload `json:"payload"`
}

type PostedPayload struct {
	Author string    `json:"author"`
	Text   string    `json:"text"`
	At     time.Time `json:"at"`
}

// PostedMessage is what everyone in the room receives.
type PostedMessage struct {
	chanx.BaseMessage `action:"posted" description:"A message posted to the room."`
	Payload           PostedPayload `json:"payload"`
}

// #endregion messages

// #region presence
type PresencePayload struct {
	User   string `json:"user"`
	Online bool   `json:"online"`
}

// PresenceMessage tells the room someone joined or left.
type PresenceMessage struct {
	chanx.BaseMessage `action:"presence"`
	Payload           PresencePayload `json:"payload"`
}

// #endregion presence

// #region consumer
type RoomConsumer struct {
	Users map[string]string // token -> user name; your user store in real life
}

// Groups puts the connection in the room named by the URL.
func (RoomConsumer) Groups(c *chanx.Conn) []string {
	return []string{"room." + c.Param("room")}
}

// HandleSend stamps the message with its author and broadcasts it to the room.
func (RoomConsumer) HandleSend(ctx *chanx.Context, m SendMessage) error {
	text := strings.TrimSpace(m.Payload.Text)
	if text == "" {
		return nil
	}
	return ctx.Broadcast(PostedMessage{Payload: PostedPayload{
		Author: ctx.Conn.User.(string),
		Text:   text,
		At:     time.Now().UTC(),
	}})
}

// #endregion consumer

// #region hooks
// OnConnect announces the newcomer to everyone already in the room.
func (RoomConsumer) OnConnect(ctx *chanx.Context) error {
	return ctx.Broadcast(PresenceMessage{Payload: PresencePayload{User: ctx.Conn.User.(string), Online: true}}, chanx.ExcludeSelf())
}

// OnDisconnect runs while the room is still joined, so the goodbye reaches it.
func (RoomConsumer) OnDisconnect(ctx *chanx.Context) {
	_ = ctx.Broadcast(PresenceMessage{Payload: PresencePayload{User: ctx.Conn.User.(string), Online: false}}, chanx.ExcludeSelf())
}

// Operations documents what HandleSend broadcasts, for the AsyncAPI document and
// the typed clients generated from it.
func (RoomConsumer) Operations() map[string]chanx.Operation {
	return map[string]chanx.Operation{
		"HandleSend": {Summary: "Post a message", Replies: []chanx.Message{PostedMessage{}}},
	}
}

// Emits declares what no handler sends: the presence messages from the hooks above.
func (RoomConsumer) Emits() []chanx.Message { return []chanx.Message{PresenceMessage{}} }

// #endregion hooks

// #region auth
// Authenticate accepts "Authorization: Bearer <token>" or "?token=<token>".
func (c RoomConsumer) Authenticate(ctx *chanx.Context, r *chanx.Request) (any, error) {
	return auth.Bearer(r, func(token string) (string, error) {
		if name, ok := c.Users[token]; ok {
			return name, nil
		}
		return "", chanx.Unauthenticated("Invalid token.")
	})
}

func (RoomConsumer) Credentials() []chanx.Credential { return auth.BearerCredentials() }

// #endregion auth

// #region main
func newEngine(set layers.Set) *chanx.Engine {
	engine := chanx.New(chanx.Config{Layers: set})
	engine.Handle("/ws/rooms/{room}", RoomConsumer{Users: map[string]string{"t-ann": "ann", "t-bob": "bob"}},
		chanx.Name("rooms"), chanx.Description("Chat rooms"))
	return engine
}

func main() {
	redisURL := flag.String("redis", os.Getenv("REDIS_URL"), "Redis URL; empty: in memory")
	addr := flag.String("addr", ":8000", "listen address")
	flag.Parse()

	var set layers.Set // nil: one in-memory layer
	if *redisURL != "" {
		layer, err := redis.NewPubSub(redis.PubSubOptions{Hosts: []string{*redisURL}})
		if err != nil {
			log.Fatal(err)
		}
		set = layers.Set{"default": layer}
	}

	engine := newEngine(set)
	mux := http.NewServeMux()
	chanxhttp.Mount(mux, engine)
	chanxhttp.MountDocs(mux, engine, "/asyncapi", asyncapi.Info{Title: "Rooms"})
	log.Printf("listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

// #endregion main
