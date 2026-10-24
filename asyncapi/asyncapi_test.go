package asyncapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	chanx "github.com/huynguyengl99/chanx-go"
	"github.com/huynguyengl99/chanx-go/asyncapi"
	"github.com/huynguyengl99/chanx-go/asyncapi/internal/otherpkg"
)

type RoomPayload struct {
	RoomName string `json:"room_name"`
}

type JoinRoom struct {
	chanx.BaseMessage `action:"join_room" description:"Join a room."`
	Payload           RoomPayload `json:"payload"`
}

type RoomJoined struct {
	chanx.BaseMessage `action:"room_joined"`
	Payload           RoomPayload `json:"payload"`
}

type LobbyConsumer struct{}

func (LobbyConsumer) HandleJoin(ctx *chanx.Context, m JoinRoom) (RoomJoined, error) {
	return RoomJoined{Payload: m.Payload}, nil
}

func engine() *chanx.Engine {
	e := chanx.New(chanx.Config{})
	e.Handle("/ws/lobby/{floor}", LobbyConsumer{})
	return e
}

func TestDocument(t *testing.T) {
	data, err := asyncapi.JSON(engine(), asyncapi.Info{})
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{
		`"asyncapi": "3.0.0"`, `"title": "AsyncAPI Documentation"`, `"development"`,
		`"lobby": {`, `"address": "/ws/lobby/{floor}"`, `"Path parameter for floor (str)"`,
		`"handle_join": {`, `"$ref": "#/channels/lobby/messages/room_joined"`,
		`"RoomPayload": {`, `"title": "Room Name"`, `"description": "Join a room."`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("document lacks %s", want)
		}
	}
	// Top-level key order: asyncapi, info, servers, channels, operations, components.
	if strings.Index(s, `"info"`) > strings.Index(s, `"channels"`) || strings.Index(s, `"operations"`) > strings.Index(s, `"components"`) {
		t.Error("top-level keys out of order")
	}
}

func TestCamelize(t *testing.T) {
	data, err := asyncapi.JSON(engine(), asyncapi.Info{Camelize: true})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	_ = json.Unmarshal(data, &doc)
	ops := doc["operations"].(map[string]any)
	if _, ok := ops["handleJoin"]; !ok {
		t.Fatalf("operations not camelized: %v", ops)
	}
	schema := doc["components"].(map[string]any)["schemas"].(map[string]any)["RoomPayload"].(map[string]any)
	if _, ok := schema["properties"].(map[string]any)["roomName"]; !ok || schema["required"].([]any)[0] != "roomName" {
		t.Fatalf("schema not camelized: %v", schema)
	}
	if !strings.Contains(string(data), `"#/channels/lobby/messages/roomJoined"`) {
		t.Error("refs not camelized")
	}
}

func TestYAML(t *testing.T) {
	data, err := asyncapi.YAML(engine(), asyncapi.Info{Title: "T"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "asyncapi: 3.0.0\ninfo:\n  title: T") {
		t.Fatalf("unexpected YAML:\n%s", data)
	}
}

func TestHandlers(t *testing.T) {
	e := engine()
	for path, c := range map[string]struct {
		h           http.Handler
		contentType string
		contains    string
	}{
		"/asyncapi.json": {asyncapi.JSONHandler(e, asyncapi.Info{}), "application/json", `"host": "docs.example:8443"`},
		"/asyncapi.yaml": {asyncapi.YAMLHandler(e, asyncapi.Info{}), "application/x-yaml", "protocol: wss"},
		"/asyncapi":      {asyncapi.HTMLHandler(e, asyncapi.Info{Title: "</script><b>"}), "text/html; charset=utf-8", "&lt;/script&gt;&lt;b&gt;"},
	} {
		req := httptest.NewRequest(http.MethodGet, "https://docs.example:8443"+path, nil)
		rec := httptest.NewRecorder()
		c.h.ServeHTTP(rec, req)
		body := rec.Body.String()
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != c.contentType || !strings.Contains(body, c.contains) {
			t.Errorf("%s: %d %s, want %q in:\n%.300s", path, rec.Code, rec.Header().Get("Content-Type"), c.contains, body)
		}
		if strings.Contains(body, "</script><b>") {
			t.Errorf("%s: unescaped title", path)
		}
	}
	var buf strings.Builder
	if err := asyncapi.Write(&buf, e, asyncapi.Info{}); err != nil || !strings.HasSuffix(buf.String(), "}\n") {
		t.Fatalf("Write: %v", err)
	}
}

type Clash struct {
	chanx.BaseMessage `action:"clash"`
	Payload           RoomPayload `json:"payload"`
}

type clashConsumer struct{}

func (clashConsumer) HandleClash(ctx *chanx.Context, m Clash) error { return nil }

// A name reused by a type of another package gets that package as a prefix;
// the first mounted keeps the plain name, on every run.
func TestSchemaNameClash(t *testing.T) {
	e := chanx.New(chanx.Config{})
	e.Handle("/a", LobbyConsumer{})
	e.Handle("/b", clashConsumer{})
	e.Handle("/c", otherpkg.Consumer{})
	first, err := asyncapi.JSON(e, asyncapi.Info{})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	_ = json.Unmarshal(first, &doc)
	schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)
	plain, _ := json.Marshal(schemas["RoomPayload"])
	other, _ := json.Marshal(schemas["OtherpkgRoomPayload"])
	if !strings.Contains(string(plain), "room_name") || !strings.Contains(string(other), "floor") {
		t.Fatalf("names not stable or not separated:\n RoomPayload %s\n OtherpkgRoomPayload %s", plain, other)
	}
	for range 20 {
		again, _ := asyncapi.JSON(e, asyncapi.Info{})
		if string(again) != string(first) {
			t.Fatal("the document changed between runs")
		}
	}
}

type Join struct {
	Room string `json:"room"`
}

type JoinRequest struct {
	chanx.BaseMessage `action:"join" title:"Join"`
	Payload           Join `json:"payload"`
}

type joinConsumer struct{}

func (joinConsumer) HandleJoin(ctx *chanx.Context, m JoinRequest) error { return nil }

// A payload type named like a message title is renamed, not dropped.
func TestPayloadNamedLikeMessage(t *testing.T) {
	e := chanx.New(chanx.Config{})
	e.Handle("/join", joinConsumer{})
	raw, err := asyncapi.JSON(e, asyncapi.Info{})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	_ = json.Unmarshal(raw, &doc)
	schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)
	ref, _ := schemas["Join"].(map[string]any)["properties"].(map[string]any)["payload"].(map[string]any)["$ref"].(string)
	payload, _ := json.Marshal(schemas[strings.TrimPrefix(ref, "#/components/schemas/")])
	if ref == "#/components/schemas/Join" || !strings.Contains(string(payload), `"room"`) {
		t.Fatalf("payload of Join is %q: %s", ref, payload)
	}
}

type Ratio struct {
	Value float64 `json:"value" default:"1.0"`
	Scale float64 `json:"scale" default:"2"`
}

type RatioMessage struct {
	chanx.BaseMessage `action:"ratio"`
	Payload           Ratio `json:"payload"`
}

type ratioConsumer struct{}

func (ratioConsumer) HandleRatio(ctx *chanx.Context, m RatioMessage) error { return nil }

// Defaults are documented as written, as Pydantic does: 1.0 stays 1.0, 2 stays 2.
func TestFloatDefaults(t *testing.T) {
	e := chanx.New(chanx.Config{})
	e.Handle("/ratio", ratioConsumer{})
	for name, render := range map[string]func(*chanx.Engine, asyncapi.Info) ([]byte, error){"json": asyncapi.JSON, "yaml": asyncapi.YAML} {
		raw, err := render(e, asyncapi.Info{})
		if err != nil {
			t.Fatal(err)
		}
		doc := strings.Join(strings.Fields(string(raw)), "")
		if !strings.Contains(doc, `default":1.0`) && !strings.Contains(doc, `default:1.0`) {
			t.Errorf("%s: default 1.0 lost", name)
		}
		if !strings.Contains(doc, `"default":2}`) && !strings.Contains(doc, `default:2`) {
			t.Errorf("%s: default 2 not kept as written", name) // Pydantic: `scale: float = 2` documents 2
		}
	}
}

type Maybe struct {
	chanx.BaseMessage `action:"maybe"`
	Payload           *RoomPayload `json:"payload,omitempty"`
}

type Named struct {
	chanx.BaseMessage `action:"named"`
	Payload           string `json:"payload" default:"anon"`
}

type optionalConsumer struct{}

func (optionalConsumer) HandleMaybe(ctx *chanx.Context, m Maybe) (Named, error) { return Named{}, nil }

// An optional payload is not required; a nullable one defaults to null, a
// defaulted one to its default.
func TestOptionalPayloads(t *testing.T) {
	e := chanx.New(chanx.Config{})
	e.Handle("/ws", optionalConsumer{})
	data, err := asyncapi.JSON(e, asyncapi.Info{})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	_ = json.Unmarshal(data, &doc)
	schemas := doc["components"].(map[string]any)["schemas"].(map[string]any)
	for name, want := range map[string]string{
		"Maybe": `{"anyOf":[{"$ref":"#/components/schemas/RoomPayload"},{"type":"null"}],"default":null,"title":"Payload"}`,
		"Named": `{"default":"anon","title":"Payload","type":"string"}`,
	} {
		s := schemas[name].(map[string]any)
		if _, required := s["required"]; required {
			t.Errorf("%s: payload marked required", name)
		}
		got, _ := json.Marshal(s["properties"].(map[string]any)["payload"])
		if string(got) != want {
			t.Errorf("%s payload: got %s, want %s", name, got, want)
		}
	}
}
