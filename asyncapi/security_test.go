package asyncapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	chanx "github.com/huynguyengl99/chanx-go"
	"github.com/huynguyengl99/chanx-go/asyncapi"
)

type SecureAsk struct {
	chanx.BaseMessage `action:"secure_ask"`
	Payload           string `json:"payload"`
}

type SecureAnswer struct {
	chanx.BaseMessage `action:"secure_answer"`
	Payload           string `json:"payload"`
}

type SecureConsumer struct{ creds []chanx.Credential }

func (c SecureConsumer) Authenticate(*chanx.Context, *chanx.Request) (any, error) { return "u", nil }
func (c SecureConsumer) Credentials() []chanx.Credential                          { return c.creds }

func (SecureConsumer) HandleSecureAsk(_ *chanx.Context, m SecureAsk) (SecureAnswer, error) {
	return SecureAnswer{Payload: m.Payload}, nil
}

var tokenCredentials = []chanx.Credential{
	{Name: "token", Location: "query", Description: "Access token"},
	{Name: "Authorization", Location: "header"},
	{Name: "sessionid", Location: "cookie"},
}

func generate(t *testing.T, creds []chanx.Credential, info asyncapi.Info) map[string]any {
	t.Helper()
	e := chanx.New(chanx.Config{})
	e.Handle("/ws/secure", SecureConsumer{creds: creds}, chanx.Name("secure_room"))
	data, err := asyncapi.JSON(e, info)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func at(v any, path ...string) any {
	for _, k := range path {
		m, _ := v.(map[string]any)
		v = m[k]
	}
	return v
}

func equalJSON(t *testing.T, what string, got any, want string) {
	t.Helper()
	var w any
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, w) {
		g, _ := json.Marshal(got)
		t.Errorf("%s:\n got %s\nwant %s", what, g, want)
	}
}

const schemeRefs = `[
	{"$ref": "#/components/securitySchemes/query_token"},
	{"$ref": "#/components/securitySchemes/header_authorization"},
	{"$ref": "#/components/securitySchemes/cookie_sessionid"}
]`

func TestCredentialsBecomeSchemesBindingsAndSecurity(t *testing.T) {
	doc := generate(t, tokenCredentials, asyncapi.Info{})
	equalJSON(t, "securitySchemes", at(doc, "components", "securitySchemes"), `{
		"query_token": {"type": "httpApiKey", "name": "token", "in": "query", "description": "Access token"},
		"header_authorization": {"type": "httpApiKey", "name": "Authorization", "in": "header"},
		"cookie_sessionid": {"type": "httpApiKey", "name": "sessionid", "in": "cookie"}
	}`)
	equalJSON(t, "bindings", at(doc, "channels", "secure_room", "bindings"), `{"ws": {
		"query": {"type": "object", "properties": {"token": {"type": "string", "description": "Access token"}}},
		"headers": {"type": "object", "properties": {"Authorization": {"type": "string"}}},
		"bindingVersion": "0.1.0"
	}}`)
	equalJSON(t, "security", at(doc, "operations", "handle_secure_ask", "security"), schemeRefs)
}

func TestCamelizedRefsKeepSchemeNames(t *testing.T) {
	doc := generate(t, tokenCredentials, asyncapi.Info{Camelize: true})
	equalJSON(t, "security", at(doc, "operations", "handleSecureAsk", "security"), schemeRefs)
	schemes := at(doc, "components", "securitySchemes").(map[string]any)
	for _, name := range []string{"query_token", "header_authorization", "cookie_sessionid"} {
		if _, ok := schemes[name]; !ok {
			t.Errorf("scheme %s renamed: %v", name, schemes)
		}
	}
}

func TestDeclaredSchemeWinsOverDerivedOne(t *testing.T) {
	declared := map[string]any{"type": "httpApiKey", "name": "token", "in": "query", "x-note": "mine"}
	doc := generate(t, tokenCredentials, asyncapi.Info{SecuritySchemes: map[string]any{
		"query_token": declared,
		"oauth":       map[string]any{"type": "oauth2"},
	}})
	equalJSON(t, "query_token", at(doc, "components", "securitySchemes", "query_token"), `{"type": "httpApiKey", "name": "token", "in": "query", "x-note": "mine"}`)
	equalJSON(t, "oauth", at(doc, "components", "securitySchemes", "oauth"), `{"type": "oauth2"}`)
}

func TestServersReplaceTheDefaultServer(t *testing.T) {
	servers := `{"production": {
		"host": "api.example.com", "protocol": "wss", "description": "Production",
		"security": [{"$ref": "#/components/securitySchemes/query_token"}]
	}}`
	var s map[string]any
	_ = json.Unmarshal([]byte(servers), &s)
	equalJSON(t, "servers", generate(t, tokenCredentials, asyncapi.Info{Servers: s})["servers"], servers)
	equalJSON(t, "default", generate(t, tokenCredentials, asyncapi.Info{})["servers"], `{"development": {"host": "localhost:8000", "protocol": "ws"}}`)
}

func TestNoCredentialsAddsNothing(t *testing.T) {
	doc := generate(t, nil, asyncapi.Info{})
	if _, ok := at(doc, "components").(map[string]any)["securitySchemes"]; ok {
		t.Error("securitySchemes without credentials")
	}
	if _, ok := at(doc, "channels", "secure_room").(map[string]any)["bindings"]; ok {
		t.Error("bindings without credentials")
	}
	if _, ok := at(doc, "operations", "handle_secure_ask").(map[string]any)["security"]; ok {
		t.Error("security without credentials")
	}
}

func TestCookieOnlyCredentialsHaveNoBinding(t *testing.T) {
	doc := generate(t, []chanx.Credential{{Name: "sessionid", Location: "cookie"}}, asyncapi.Info{})
	equalJSON(t, "securitySchemes", at(doc, "components", "securitySchemes"), `{"cookie_sessionid": {"type": "httpApiKey", "name": "sessionid", "in": "cookie"}}`)
	if _, ok := at(doc, "channels", "secure_room").(map[string]any)["bindings"]; ok {
		t.Error("binding for a cookie")
	}
	equalJSON(t, "security", at(doc, "operations", "handle_secure_ask", "security"), `[{"$ref": "#/components/securitySchemes/cookie_sessionid"}]`)
}

type securedRoomTopic struct {
	chanx.BaseTopic `pattern:"room:{room}"`
}

func (securedRoomTopic) HandleSecureAsk(_ *chanx.Context, m SecureAsk) error { return nil }

// A hosted topic is reached through its host's handshake, so it carries the
// host's credentials; a function consumer declares them as a field.
func TestHostedTopicUsesHostCredentials(t *testing.T) {
	hub := chanx.NewConsumer("hub")
	hub.Authenticate = func(*chanx.Context, *chanx.Request) (any, error) { return "u", nil }
	hub.Credentials = []chanx.Credential{{Name: "X-Api-Key", Location: "header"}}
	hub.Topics = []chanx.TopicHandler{securedRoomTopic{}}
	e := chanx.New(chanx.Config{})
	e.Handle("/ws/hub", hub)
	data, err := asyncapi.JSON(e, asyncapi.Info{})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	_ = json.Unmarshal(data, &doc)
	ref := `[{"$ref": "#/components/securitySchemes/header_x_api_key"}]`
	for name, op := range at(doc, "operations").(map[string]any) {
		equalJSON(t, name, at(op, "security"), ref)
	}
	for name, ch := range at(doc, "channels").(map[string]any) {
		equalJSON(t, name, at(ch, "bindings", "ws", "headers", "properties"), `{"X-Api-Key": {"type": "string"}}`)
	}
}

func TestCorrelationIDs(t *testing.T) {
	doc := generate(t, nil, asyncapi.Info{})
	for name, title := range map[string]string{"secure_ask": "SecureAsk", "secure_answer": "SecureAnswer"} {
		equalJSON(t, name, at(doc, "components", "messages", name), `{
			"payload": {"$ref": "#/components/schemas/`+title+`"},
			"correlationId": {"$ref": "#/components/correlationIds/ref"}
		}`)
	}
	equalJSON(t, "correlationIds", at(doc, "components", "correlationIds"), `{"ref": {
		"description": "A reply carries the `+"`ref`"+` of the request it answers.",
		"location": "$message.payload#/ref"
	}}`)

	// A handler without a reply correlates nothing.
	e := chanx.New(chanx.Config{})
	e.Handle("/b", clashConsumer{})
	data, _ := asyncapi.JSON(e, asyncapi.Info{})
	if strings.Contains(string(data), "correlation") {
		t.Errorf("correlation without a reply:\n%s", data)
	}
}

func TestInfoDescription(t *testing.T) {
	if _, ok := at(generate(t, nil, asyncapi.Info{}), "info").(map[string]any)["description"]; ok {
		t.Error("empty description written")
	}
	if d := at(generate(t, nil, asyncapi.Info{Description: "Chat"}), "info", "description"); d != "Chat" {
		t.Errorf("description %v", d)
	}
}

func TestDocsAssets(t *testing.T) {
	page := func(info asyncapi.Info) string {
		rec := httptest.NewRecorder()
		asyncapi.HTMLHandler(chanx.New(chanx.Config{}), info).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/asyncapi", nil))
		return rec.Body.String()
	}
	pinned := page(asyncapi.Info{})
	for _, want := range []string{
		`<script src="https://unpkg.com/@asyncapi/react-component@3.2.2/browser/standalone/index.js" integrity="sha384-/OTzBCE2fCs+wXFZecqj96eHeOnof8fcoSA2OVEHTmV/BeGnpOpolyhDGduFU9xE" crossorigin="anonymous"></script>`,
		`<link rel="stylesheet" href="https://unpkg.com/@asyncapi/react-component@3.2.2/styles/default.min.css" integrity="sha384-oo9RoQcacP++XdMX6CjTucTvASEORHX3chFik0/V2kHcsHiVboGyWZztGeq/0bum" crossorigin="anonymous">`,
	} {
		if !strings.Contains(pinned, want) {
			t.Errorf("default page lacks %s", want)
		}
	}

	own := page(asyncapi.Info{DocsJSURL: "/static/ui.js", DocsCSSURL: `/static/"ui".css`, DocsCSSIntegrity: "sha384-mine"})
	for _, want := range []string{
		`<script src="/static/ui.js"></script>`,
		`<link rel="stylesheet" href="/static/&#34;ui&#34;.css" integrity="sha384-mine" crossorigin="anonymous">`,
	} {
		if !strings.Contains(own, want) {
			t.Errorf("custom page lacks %s:\n%s", want, own)
		}
	}
}
