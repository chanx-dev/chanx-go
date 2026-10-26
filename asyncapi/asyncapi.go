// Package asyncapi generates the AsyncAPI 3.0 document of a chanx engine:
// channels with x-topic, receive and send operations with replies, and component
// schemas, in the shape every chanx client generator reads.
package asyncapi

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/google/jsonschema-go/jsonschema"

	chanx "go.chanx.dev"
	"go.chanx.dev/internal/schema"
	"go.chanx.dev/protocol"
)

// Info is the document metadata.
type Info struct {
	Title       string // default "AsyncAPI Documentation"
	Version     string // default "1.0.0"
	Description string
	// ServerURL is the server host ("localhost:8000"); the handlers default it to
	// the request's Host.
	ServerURL string
	// ServerProtocol is "ws" or "wss"; the handlers default it from the request.
	ServerProtocol string
	// Servers are AsyncAPI Server Objects by name, replacing ServerURL and
	// ServerProtocol.
	Servers map[string]any
	// SecuritySchemes are AsyncAPI Security Scheme Objects by name, added to the
	// ones derived from the consumers' credentials; a name declared here wins.
	SecuritySchemes map[string]any
	// Camelize camelizes channel, operation and message names and schema
	// properties.
	Camelize bool

	// DocsJSURL and DocsCSSURL serve the docs page assets from elsewhere, such as
	// your own static files; they default to a pinned CDN release.
	DocsJSURL  string
	DocsCSSURL string
	// DocsJSIntegrity and DocsCSSIntegrity are the assets' SRI hashes; the pinned
	// defaults only apply to the default URLs.
	DocsJSIntegrity  string
	DocsCSSIntegrity string
}

func (i Info) withDefaults() Info {
	if i.Title == "" {
		i.Title = "AsyncAPI Documentation"
	}
	if i.Version == "" {
		i.Version = "1.0.0"
	}
	if i.ServerURL == "" {
		i.ServerURL = "localhost:8000"
	}
	if i.ServerProtocol == "" {
		i.ServerProtocol = "ws"
	}
	return i
}

// entry is one documented channel: a consumer, or a topic on a route.
type entry struct {
	path       string
	params     []string
	route      chanx.RouteInfo
	consumer   chanx.ConsumerInfo
	topic      *chanx.TopicInfo
	hostSnake  string // set for a topic hosted by a consumer
	channel    string
	standalone bool
}

type generator struct {
	info       Info
	schemas    *schema.Generator
	msgs       map[reflect.Type]*chanx.MessageInfo
	msgOrder   []*chanx.MessageInfo // first seen first: names must not depend on map order
	byConsumer map[string]map[reflect.Type]*chanx.MessageInfo
	opNames    map[string]bool
	correlated map[string]bool // component messages of request/reply operations
	schemes    *Map
	security   map[string][]any // scheme $refs by channel
}

// Generate builds the document.
func Generate(e *chanx.Engine, info Info) (*Map, error) {
	info = info.withDefaults()
	g := &generator{
		info:       info,
		schemas:    schema.NewGenerator("#/components/schemas/"),
		msgs:       map[reflect.Type]*chanx.MessageInfo{},
		byConsumer: map[string]map[reflect.Type]*chanx.MessageInfo{},
		opNames:    map[string]bool{},
		correlated: map[string]bool{},
		schemes:    NewMap(),
		security:   map[string][]any{},
	}
	for _, name := range slices.Sorted(maps.Keys(info.SecuritySchemes)) {
		s, err := toOrdered(info.SecuritySchemes[name])
		if err != nil {
			return nil, err
		}
		g.schemes.Set(name, s)
	}
	g.schemas.Rename = func(t reflect.Type, name string) string {
		pkg := t.PkgPath()
		if i := strings.LastIndex(pkg, "/"); i >= 0 {
			pkg = pkg[i+1:]
		}
		return pascal(pkg) + name
	}

	var entries []*entry
	for _, r := range e.Routes() {
		ri := r.Info()
		if ri.Standalone != nil {
			entries = append(entries, &entry{path: ri.Path, params: ri.ParamNames, route: ri, consumer: ri.Standalone.ConsumerInfo, topic: ri.Standalone, standalone: true})
			continue
		}
		if len(ri.Topics) == 0 || len(ri.Consumer.Handlers) > 0 {
			entries = append(entries, &entry{path: ri.Path, params: ri.ParamNames, route: ri, consumer: ri.Consumer})
		}
		for i := range ri.Topics {
			t := ri.Topics[i]
			entries = append(entries, &entry{path: ri.Path, params: ri.ParamNames, route: ri, consumer: t.ConsumerInfo, topic: &t, hostSnake: ri.Consumer.SnakeName})
		}
	}

	for _, en := range entries {
		g.register(en.consumer)
	}

	channels := NewMap()
	for _, en := range entries {
		name, ch, err := g.channel(en)
		if err != nil {
			return nil, err
		}
		en.channel = name
		channels.Set(name, ch)
	}
	operations := NewMap()
	for _, en := range entries {
		for _, h := range en.consumer.Handlers {
			g.operation(operations, en, h, false)
		}
		for _, h := range en.consumer.Events {
			g.operation(operations, en, h, true)
		}
	}

	messages := NewMap()
	schemas := NewMap()
	for _, info := range g.msgOrder { // message schemas share the namespace with payload types
		g.schemas.Reserve(info.Title, info.Type)
	}
	for _, info := range g.msgOrder {
		messages.Set(protocol.Depascalize(info.Title), NewMap().Set("payload", NewMap().Set("$ref", "#/components/schemas/"+info.Title)))
		ms, err := g.messageSchema(info)
		if err != nil {
			return nil, err
		}
		schemas.Set(info.Title, ms)
	}
	for _, name := range g.schemas.Order {
		if schemas.Has(name) {
			continue
		}
		s, err := toOrdered(g.schemas.Defs[name])
		if err != nil {
			return nil, err
		}
		schemas.Set(name, s)
	}

	servers, err := g.servers()
	if err != nil {
		return nil, err
	}
	components := NewMap().Set("messages", messages.Sorted()).Set("schemas", schemas.Sorted())
	g.addCorrelationIDs(components)
	if g.schemes.Len() > 0 {
		components.Set("securitySchemes", g.schemes)
	}
	meta := NewMap().Set("title", info.Title).Set("version", info.Version)
	if info.Description != "" {
		meta.Set("description", info.Description)
	}
	doc := NewMap().
		Set("asyncapi", "3.0.0").
		Set("info", meta).
		Set("servers", servers).
		Set("channels", channels).
		Set("operations", operations).
		Set("components", components)
	if info.Camelize {
		camelizeDoc(doc)
	}
	return doc, nil
}

func (g *generator) servers() (*Map, error) {
	if len(g.info.Servers) == 0 {
		name := "production"
		for _, local := range []string{"localhost", "127.0.0.1", "0.0.0.0", "::1", "[::1]"} {
			if strings.Contains(strings.ToLower(g.info.ServerURL), local) {
				name = "development"
			}
		}
		return NewMap().Set(name, NewMap().Set("host", g.info.ServerURL).Set("protocol", g.info.ServerProtocol)), nil
	}
	out := NewMap()
	for _, name := range slices.Sorted(maps.Keys(g.info.Servers)) {
		s, err := toOrdered(g.info.Servers[name])
		if err != nil {
			return nil, err
		}
		out.Set(name, s)
	}
	return out, nil
}

// addCorrelationIDs marks request and reply messages as correlated by the
// frame's ref.
func (g *generator) addCorrelationIDs(components *Map) {
	if len(g.correlated) == 0 {
		return
	}
	messages, _ := get[*Map](components, "messages")
	for _, name := range messages.Keys() {
		if g.correlated[name] {
			m, _ := get[*Map](messages, name)
			m.Set("correlationId", NewMap().Set("$ref", "#/components/correlationIds/ref"))
		}
	}
	components.Set("correlationIds", NewMap().Set("ref", NewMap().
		Set("description", "A reply carries the `ref` of the request it answers.").
		Set("location", "$message.payload#/ref")))
}

// addSecurityScheme registers a credential's scheme, keeping a declared one,
// and returns its name.
func (g *generator) addSecurityScheme(c chanx.Credential) string {
	name := strings.ReplaceAll(strings.ToLower(c.Location+"_"+c.Name), "-", "_")
	if !g.schemes.Has(name) {
		s := NewMap().Set("type", "httpApiKey").Set("name", c.Name).Set("in", c.Location)
		if c.Description != "" {
			s.Set("description", c.Description)
		}
		g.schemes.Set(name, s)
	}
	return name
}

// wsBinding documents the credentials sent in the handshake's query or headers.
func wsBinding(creds []chanx.Credential) *Map {
	binding := NewMap()
	for _, part := range [][2]string{{"query", "query"}, {"headers", "header"}} {
		props := NewMap()
		for _, c := range creds {
			if c.Location == part[1] {
				p := NewMap().Set("type", "string")
				if c.Description != "" {
					p.Set("description", c.Description)
				}
				props.Set(c.Name, p)
			}
		}
		if props.Len() > 0 {
			binding.Set(part[0], NewMap().Set("type", "object").Set("properties", props))
		}
	}
	if binding.Len() == 0 {
		return nil
	}
	return binding.Set("bindingVersion", "0.1.0")
}

func pascal(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func (g *generator) add(consumer string, infos ...*chanx.MessageInfo) {
	set := g.byConsumer[consumer]
	if set == nil {
		set = map[reflect.Type]*chanx.MessageInfo{}
		g.byConsumer[consumer] = set
	}
	for _, i := range infos {
		if i == nil {
			continue
		}
		set[i.Type] = i
		if _, seen := g.msgs[i.Type]; !seen {
			g.msgOrder = append(g.msgOrder, i)
		}
		g.msgs[i.Type] = i
	}
}

// register records the messages a consumer's channel lists: handler inputs and
// replies, event outputs and Emits (event inputs travel server to server only).
func (g *generator) register(c chanx.ConsumerInfo) {
	for _, h := range c.Handlers {
		g.add(c.TypeName, h.Input)
		g.add(c.TypeName, h.Outputs...)
	}
	for _, h := range c.Events {
		g.add(c.TypeName, h.Outputs...)
	}
	g.add(c.TypeName, c.Emits...)
}

func (g *generator) channel(en *entry) (string, *Map, error) {
	name := en.route.Name
	if en.topic != nil && !en.standalone {
		name = en.hostSnake + "_" + en.consumer.SnakeName
	} else if name == "" {
		name = en.consumer.SnakeName
	}
	description := en.route.Description
	if en.topic != nil && (description == "" || !en.standalone) {
		description = en.topic.Description
	}
	ch := NewMap().Set("address", en.path).Set("title", name)
	if description != "" {
		ch.Set("description", description)
	}
	if len(en.params) > 0 {
		params := NewMap()
		for _, p := range en.params {
			params.Set(p, NewMap().Set("description", fmt.Sprintf("Path parameter for %s (str)", p)))
		}
		ch.Set("parameters", params)
	}
	if en.topic != nil {
		ch.Set("x-topic", NewMap().
			Set("name", protocol.Decamelize(en.topic.Namespace)).
			Set("pattern", en.topic.Pattern).
			Set("parameters", append([]string{}, en.topic.Params...)))
	}
	infos := g.byConsumer[en.consumer.TypeName]
	sorted := make([]*chanx.MessageInfo, 0, len(infos))
	for _, i := range infos {
		sorted = append(sorted, i)
	}
	sort.Slice(sorted, func(a, b int) bool { return sorted[a].Type.Name() < sorted[b].Type.Name() })
	msgs := NewMap()
	for _, i := range sorted {
		key := protocol.Decamelize(i.Title)
		msgs.Set(key, NewMap().Set("$ref", "#/components/messages/"+protocol.Depascalize(i.Title)))
	}
	ch.Set("messages", msgs.Sorted())
	if creds := en.route.Credentials; len(creds) > 0 {
		for _, c := range creds {
			g.security[name] = append(g.security[name], NewMap().Set("$ref", "#/components/securitySchemes/"+g.addSecurityScheme(c)))
		}
		if b := wsBinding(creds); b != nil {
			ch.Set("bindings", NewMap().Set("ws", b))
		}
	}
	tags := en.route.Tags
	if en.topic != nil && !en.standalone {
		tags = nil
	}
	if len(tags) > 0 {
		var ts []any
		for _, t := range tags {
			ts = append(ts, NewMap().Set("name", t))
		}
		ch.Set("tags", ts)
	}
	return name, ch, nil
}

func (g *generator) operation(ops *Map, en *entry, h chanx.HandlerInfo, event bool) {
	name := h.Operation
	if g.opNames[name] {
		name = en.consumer.SnakeName + "_" + name
	}
	g.opNames[name] = true
	chRef := NewMap().Set("$ref", "#/channels/"+en.channel)
	action := "receive"
	if event {
		action = "send"
	}
	op := NewMap().Set("action", action).Set("channel", chRef).Set("description", h.Description).Set("summary", h.Summary)
	if sec := g.security[en.channel]; len(sec) > 0 {
		op.Set("security", sec)
	}
	if len(h.Tags) > 0 {
		var ts []any
		for _, t := range h.Tags {
			ts = append(ts, NewMap().Set("name", t))
		}
		op.Set("tags", ts)
	}
	ref := func(i *chanx.MessageInfo) *Map {
		return NewMap().Set("$ref", "#/channels/"+en.channel+"/messages/"+protocol.Decamelize(i.Title))
	}
	if !event {
		op.Set("messages", []any{ref(h.Input)})
	}
	if len(h.Outputs) > 0 {
		var outs []any
		for _, o := range h.Outputs {
			outs = append(outs, ref(o))
		}
		if event {
			op.Set("messages", outs)
		} else {
			op.Set("reply", NewMap().Set("channel", NewMap().Set("$ref", "#/channels/"+en.channel)).Set("messages", outs))
			g.correlated[protocol.Depascalize(h.Input.Title)] = true
			for _, o := range h.Outputs {
				g.correlated[protocol.Depascalize(o.Title)] = true
			}
		}
	}
	ops.Set(name, op)
}

// messageSchema is the schema of a message: action as a constant,
// payload as a $ref to its model or inline.
func (g *generator) messageSchema(info *chanx.MessageInfo) (*Map, error) {
	action := NewMap().Set("const", info.Action).Set("default", info.Action).Set("title", "Action").Set("type", "string")
	var payload any
	if info.PayloadType == nil {
		payload = NewMap().Set("default", nil).Set("title", "Payload").Set("type", "null")
	} else {
		s := g.schemas.For(info.PayloadType)
		if s.Ref != "" {
			s = &jsonschema.Schema{Ref: s.Ref}
		} else {
			cp := *s
			s = &cp
			s.Title = "Payload"
		}
		switch {
		case info.HasDefault:
			s.Default = schema.DefaultJSON(info.PayloadType, info.PayloadDefault)
		case info.PayloadOptional && (info.PayloadType.Kind() == reflect.Interface || info.PayloadType.Kind() == reflect.Pointer):
			s.Default = json.RawMessage("null")
		}
		p, err := toOrdered(s)
		if err != nil {
			return nil, err
		}
		if b, ok := p.(bool); ok && b {
			p = NewMap()
		}
		payload = p
	}
	m := NewMap()
	if info.Description != "" {
		m.Set("description", info.Description)
	}
	m.Set("properties", NewMap().Set("action", action).Set("payload", payload))
	if info.PayloadType != nil && !info.PayloadOptional {
		m.Set("required", []any{"payload"})
	}
	m.Set("title", info.Title).Set("type", "object")
	return m, nil
}

// camelizeDoc camelizes names and schema properties; schema names stay type
// names.
func camelizeDoc(doc *Map) {
	rename := func(m *Map, fn func(string, any) any) *Map {
		out := NewMap()
		for _, k := range m.Keys() {
			v, _ := m.Get(k)
			out.Set(protocol.Camelize(k), fn(k, v))
		}
		return out
	}
	same := func(_ string, v any) any { return v }
	if ch, ok := get[*Map](doc, "channels"); ok {
		doc.Set("channels", rename(ch, func(_ string, v any) any {
			c := v.(*Map)
			if msgs, ok := get[*Map](c, "messages"); ok {
				c.Set("messages", rename(msgs, same))
			}
			return c
		}))
	}
	if ops, ok := get[*Map](doc, "operations"); ok {
		doc.Set("operations", rename(ops, same))
	}
	if comps, ok := get[*Map](doc, "components"); ok {
		if msgs, ok := get[*Map](comps, "messages"); ok {
			comps.Set("messages", rename(msgs, same))
		}
		if schemas, ok := get[*Map](comps, "schemas"); ok {
			out := NewMap()
			for _, k := range schemas.Keys() {
				v, _ := schemas.Get(k)
				out.Set(k, camelizeSchema(v))
			}
			comps.Set("schemas", out)
		}
	}
	camelizeRefs(doc)
}

func get[T any](m *Map, k string) (T, bool) {
	v, ok := m.Get(k)
	t, ok2 := v.(T)
	return t, ok && ok2
}

// camelizeSchema is humps.camelize over every key, plus the names in required.
func camelizeSchema(v any) any {
	switch x := v.(type) {
	case *Map:
		out := NewMap()
		for _, k := range x.Keys() {
			val, _ := x.Get(k)
			if k == "required" {
				if list, ok := val.([]any); ok {
					req := make([]any, len(list))
					for i, r := range list {
						if s, ok := r.(string); ok {
							req[i] = protocol.Camelize(s)
						} else {
							req[i] = r
						}
					}
					out.Set(protocol.Camelize(k), req)
					continue
				}
			}
			out.Set(protocol.Camelize(k), camelizeSchema(val))
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = camelizeSchema(e)
		}
		return out
	}
	return v
}

func camelizeRefs(v any) {
	switch x := v.(type) {
	case *Map:
		for _, k := range x.Keys() {
			val, _ := x.Get(k)
			if s, ok := val.(string); ok && k == "$ref" {
				x.Set(k, camelizeRef(s))
				continue
			}
			camelizeRefs(val)
		}
	case []any:
		for _, e := range x {
			camelizeRefs(e)
		}
	}
}

func camelizeRef(ref string) string {
	parts := strings.Split(ref, "/")
	for i, p := range parts {
		switch {
		case i <= 1, p == "channels", p == "messages", p == "operations", p == "components", p == "schemas", p == "securitySchemes":
		case parts[i-1] == "schemas", parts[i-1] == "securitySchemes": // names kept as declared
		default:
			parts[i] = protocol.Camelize(p)
		}
	}
	return strings.Join(parts, "/")
}

// JSON renders the document as JSON.
func JSON(e *chanx.Engine, info Info) ([]byte, error) {
	doc, err := Generate(e, info)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(doc, "", "  ")
}

// YAML renders the document as YAML.
func YAML(e *chanx.Engine, info Info) ([]byte, error) {
	doc, err := Generate(e, info)
	if err != nil {
		return nil, err
	}
	return yaml.Marshal(doc)
}

// Write writes the JSON document, for a `-asyncapi` flag in a main package.
func Write(w io.Writer, e *chanx.Engine, info Info) error {
	data, err := JSON(e, info)
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

func fromRequest(r *http.Request, info Info) Info {
	if info.ServerURL == "" {
		info.ServerURL = r.Host
	}
	if info.ServerProtocol == "" {
		info.ServerProtocol = "ws"
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			info.ServerProtocol = "wss"
		}
	}
	return info
}

// JSONHandler serves the document as JSON.
func JSONHandler(e *chanx.Engine, info Info) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := JSON(e, fromRequest(r, info))
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": %q}`, "Failed to generate schema: "+err.Error()), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	})
}

// YAMLHandler serves the document as YAML.
func YAMLHandler(e *chanx.Engine, info Info) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := YAML(e, fromRequest(r, info))
		if err != nil {
			http.Error(w, "Failed to generate schema: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/x-yaml")
		_, _ = w.Write(data)
	})
}

// HTMLHandler serves the AsyncAPI React viewer with the document inlined.
func HTMLHandler(e *chanx.Engine, info Info) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := fromRequest(r, info).withDefaults()
		doc, err := Generate(e, i)
		var data []byte
		if err == nil {
			data, err = json.Marshal(doc)
		}
		if err != nil {
			http.Error(w, "Failed to generate schema: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// Inside <script>, "</" must not end the element.
		safe := strings.ReplaceAll(string(data), "</", `<\/`)
		js, css := docsAsset(i.DocsJSURL, i.DocsJSIntegrity, defaultDocsJS), docsAsset(i.DocsCSSURL, i.DocsCSSIntegrity, defaultDocsCSS)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprintf(w, htmlPage, html.EscapeString(i.Title), css, js, safe)
	})
}

// The docs page pins one @asyncapi/react-component release with SRI hashes.
const (
	docsUIVersion  = "3.2.2"
	docsCDN        = "https://unpkg.com/@asyncapi/react-component@" + docsUIVersion
	defaultDocsJS  = docsCDN + "/browser/standalone/index.js"
	defaultDocsCSS = docsCDN + "/styles/default.min.css"
)

// A hash only matches its exact file, so it applies to the default URLs alone.
var defaultDocsIntegrity = map[string]string{
	defaultDocsJS:  "sha384-/OTzBCE2fCs+wXFZecqj96eHeOnof8fcoSA2OVEHTmV/BeGnpOpolyhDGduFU9xE",
	defaultDocsCSS: "sha384-oo9RoQcacP++XdMX6CjTucTvASEORHX3chFik0/V2kHcsHiVboGyWZztGeq/0bum",
}

// docsAsset returns an asset's escaped URL followed by its integrity
// attributes, if it has a hash.
func docsAsset(url, integrity, fallback string) string {
	if url == "" {
		url = fallback
	}
	if integrity == "" {
		integrity = defaultDocsIntegrity[url]
	}
	out := `"` + html.EscapeString(url) + `"`
	if integrity != "" {
		out += ` integrity="` + html.EscapeString(integrity) + `" crossorigin="anonymous"`
	}
	return out
}

const htmlPage = `<!DOCTYPE html>
<html>
    <head>
        <title>%s</title>
        <link rel="stylesheet" href=%s>
    </head>
    <body>
        <div id="asyncapi"></div>
        <script src=%s></script>
        <script>
            AsyncApiStandalone.render({
                schema: %s,
                config: {
                    show: {
                        sidebar: true,
                    }
                },
            }, document.getElementById('asyncapi'));
        </script>
    </body>
</html>
`
