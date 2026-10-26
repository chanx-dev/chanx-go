// Package codegen generates a typed Go client from a chanx AsyncAPI 3 document,
// for package chanxclient. cmd/chanx-codegen is its command line.
package codegen

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"go/token"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Options configures Generate.
type Options struct {
	// Package is the generated package's name.
	Package string
}

// Generate returns the generated files by name: messages.go and channels.go.
func Generate(doc []byte, opts Options) (map[string][]byte, error) {
	var d document
	if err := json.Unmarshal(doc, &d); err != nil {
		return nil, fmt.Errorf("codegen: %w", err)
	}
	if !strings.HasPrefix(d.AsyncAPI, "3.") {
		return nil, fmt.Errorf("codegen: AsyncAPI %q is not 3.x", d.AsyncAPI)
	}
	if len(d.Channels) == 0 {
		return nil, errors.New("codegen: the document has no channels")
	}
	if opts.Package == "" {
		opts.Package = "chanxgen"
	}
	if !token.IsIdentifier(opts.Package) {
		return nil, fmt.Errorf("codegen: package name %q is not a Go identifier", opts.Package)
	}
	g := &generator{doc: &d, pkg: opts.Package, names: map[string]string{}}
	conns := g.analyze()
	if err := g.claimNames(conns); err != nil {
		return nil, err
	}
	messages, err := g.emitMessages()
	if err != nil {
		return nil, err
	}
	channels := g.emitChannels(conns)
	out := map[string][]byte{}
	for name, src := range map[string][]byte{"messages.go": messages, "channels.go": channels} {
		formatted, err := format.Source(src)
		if err != nil {
			return nil, fmt.Errorf("codegen: %s does not parse: %w\n%s", name, err, src)
		}
		out[name] = formatted
	}
	return out, nil
}

type generator struct {
	doc   *document
	pkg   string
	names map[string]string // Go identifier -> what claimed it
	extra []namedType       // inline objects lifted to named types
}

type namedType struct {
	name   string
	schema *Schema
}

// conn is a channel the client dials, with the topics multiplexed over it.
type conn struct {
	key, name string
	ch        *channel
	flow      flow
	topics    []topic
}

type topic struct {
	key, name string
	ch        *channel
	flow      flow
}

// flow is what travels on a channel, as schema names.
type flow struct {
	toServer []string
	toClient []string
	replies  map[string][]string // request schema -> reply schemas
}

func (g *generator) analyze() []conn {
	flows := map[string]*flow{}
	at := func(key string) *flow {
		if flows[key] == nil {
			flows[key] = &flow{replies: map[string][]string{}}
		}
		return flows[key]
	}
	for _, name := range sortedKeys(g.doc.Operations) {
		op := g.doc.Operations[name]
		f := at(op.Channel.name())
		switch op.Action {
		case "receive":
			for _, m := range op.Messages {
				req := g.schemaOf(m)
				f.toServer = appendNew(f.toServer, req)
				if op.Reply != nil {
					for _, r := range op.Reply.Messages {
						rep := g.schemaOf(r)
						f.toClient = appendNew(f.toClient, rep)
						f.replies[req] = appendNew(f.replies[req], rep)
					}
				}
			}
		case "send":
			for _, m := range op.Messages {
				f.toClient = appendNew(f.toClient, g.schemaOf(m))
			}
		}
	}
	// A message the channel lists but no operation names, such as a broadcast
	// declared with Emits, still reaches the client.
	for _, key := range sortedKeys(g.doc.Channels) {
		f := at(key)
		for _, name := range sortedKeys(g.doc.Channels[key].Messages) {
			if s := g.schemaOf(ref{Ref: name}); !slices.Contains(f.toServer, s) {
				f.toClient = appendNew(f.toClient, s)
			}
		}
	}
	plain := map[string]string{} // address -> plain channel key
	for _, key := range sortedKeys(g.doc.Channels) {
		if g.doc.Channels[key].Topic == nil {
			plain[g.doc.Channels[key].Address] = key
		}
	}
	var conns []conn
	byKey := map[string]int{}
	for _, key := range sortedKeys(g.doc.Channels) {
		ch := g.doc.Channels[key]
		if host, ok := plain[ch.Address]; ok && ch.Topic != nil && host != key {
			continue
		}
		byKey[key] = len(conns)
		conns = append(conns, conn{key: key, name: goName(channelName(key, ch)), ch: ch, flow: *at(key)})
	}
	for _, key := range sortedKeys(g.doc.Channels) {
		ch := g.doc.Channels[key]
		host, ok := plain[ch.Address]
		if ch.Topic == nil || !ok || host == key {
			continue
		}
		c := &conns[byKey[host]]
		c.topics = append(c.topics, topic{key: key, name: goName(channelName(key, ch)), ch: ch, flow: *at(key)})
	}
	return conns
}

func channelName(key string, ch *channel) string {
	if ch.Title != "" {
		return ch.Title
	}
	return key
}

// schemaOf resolves an operation's message ref to its payload schema's name.
func (g *generator) schemaOf(r ref) string {
	return g.doc.Components.Messages[r.name()].Payload.name()
}

func (g *generator) claim(name, by string) error {
	if prev, ok := g.names[name]; ok {
		return fmt.Errorf("codegen: generated names would clash: %s is both %s and %s. Rename the channels, topics or schemas on the server", name, prev, by)
	}
	g.names[name] = by
	return nil
}

func (g *generator) claimNames(conns []conn) error {
	for _, name := range sortedKeys(g.doc.Components.Schemas) {
		if err := g.claim(goName(name), "schema "+name); err != nil {
			return err
		}
	}
	for _, c := range conns {
		by := "channel " + c.key
		for _, n := range []string{c.name + "Channel", c.name + "Client", c.name + "ToServer", "Dial" + c.name} {
			if err := g.claim(n, by); err != nil {
				return err
			}
		}
		methods := map[string]string{"Conn": "", "Close": "", "Send": "", "Receive": ""}
		for _, req := range c.flow.toServer {
			methods["Request"+goName(g.actionOf(req))] = req
		}
		for _, t := range c.topics {
			for _, n := range []string{t.name, t.name + "ToServer"} {
				if err := g.claim(n, "topic "+t.key); err != nil {
					return err
				}
			}
			m := goName(t.ch.Topic.Name)
			if _, taken := methods[m]; taken {
				return fmt.Errorf("codegen: generated names would clash: %sClient.%s is both a method and topic %s. Rename the topic on the server", c.name, m, t.key)
			}
			methods[m] = t.key
		}
	}
	return nil
}

func (g *generator) isMessage(name string) bool {
	s := g.doc.Components.Schemas[name]
	return s != nil && s.Properties.values["action"] != nil && len(s.Properties.values["action"].Const) > 0
}

func (g *generator) emitMessages() ([]byte, error) {
	var b bytes.Buffer
	header(&b, g.pkg)
	b.WriteString("import chanx \"github.com/huynguyengl99/chanx-go\"\n\n")
	for _, name := range sortedKeys(g.doc.Components.Schemas) {
		s := g.doc.Components.Schemas[name]
		var err error
		switch {
		case g.isMessage(name):
			err = g.emitMessage(&b, name, s)
		case len(s.Enum) > 0:
			g.emitEnum(&b, goName(name), s)
		default:
			err = g.emitType(&b, goName(name), s)
		}
		if err != nil {
			return nil, err
		}
	}
	for i := 0; i < len(g.extra); i++ { // emitting may lift more
		if err := g.emitType(&b, g.extra[i].name, g.extra[i].schema); err != nil {
			return nil, err
		}
	}
	return b.Bytes(), nil
}

func header(b *bytes.Buffer, pkg string) {
	fmt.Fprintf(b, "// Code generated by chanx-codegen. DO NOT EDIT.\n\npackage %s\n\n", pkg)
}

func comment(b *bytes.Buffer, text, indent string) {
	for line := range strings.SplitSeq(strings.TrimSpace(text), "\n") {
		fmt.Fprintf(b, "%s%s\n", indent, strings.TrimRight("// "+strings.TrimSpace(line), " "))
	}
}

func (g *generator) emitMessage(b *bytes.Buffer, name string, s *Schema) error {
	var action string
	if err := json.Unmarshal(s.Properties.values["action"].Const, &action); err != nil {
		return fmt.Errorf("codegen: %s: action is not a string", name)
	}
	if s.Description != "" {
		comment(b, s.Description, "")
	}
	typ := goName(name)
	fmt.Fprintf(b, "type %s struct {\n\tchanx.BaseMessage `action:%s`\n", typ, strconv.Quote(action))
	if p := s.Properties.values["payload"]; p != nil && !isNull(p) {
		if err := g.emitField(b, typ, "payload", p, slices.Contains(s.Required, "payload")); err != nil {
			return err
		}
	}
	b.WriteString("}\n\n")
	return nil
}

func (g *generator) emitEnum(b *bytes.Buffer, name string, s *Schema) {
	base := "string"
	if len(s.Type) == 1 {
		base = scalar(s.Type[0])
	}
	if s.Description != "" {
		comment(b, s.Description, "")
	}
	fmt.Fprintf(b, "type %s %s\n\nconst (\n", name, base)
	for _, v := range s.Enum {
		lit, _ := json.Marshal(v)
		fmt.Fprintf(b, "\t%s %s = %s\n", name+enumName(fmt.Sprint(v)), name, lit)
	}
	b.WriteString(")\n\n")
}

func (g *generator) emitType(b *bytes.Buffer, name string, s *Schema) error {
	if s.Description != "" {
		comment(b, s.Description, "")
	}
	if !isObject(s) {
		typ, err := g.goType(s, name, "")
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "type %s %s\n\n", name, typ)
		return nil
	}
	fmt.Fprintf(b, "type %s struct {\n", name)
	for _, key := range s.Properties.keys {
		if err := g.emitField(b, name, key, s.Properties.values[key], slices.Contains(s.Required, key)); err != nil {
			return err
		}
	}
	b.WriteString("}\n\n")
	return nil
}

func (g *generator) emitField(b *bytes.Buffer, owner, key string, s *Schema, required bool) error {
	field := goName(key)
	typ, err := g.goType(s, owner, field)
	if err != nil {
		return err
	}
	tag := key
	if !required {
		tag += ",omitzero"
	}
	tags := "json:" + strconv.Quote(tag)
	if def, ok := defaultTag(s, typ); ok {
		tags += " default:" + strconv.Quote(def)
	}
	if s.Description != "" {
		comment(b, s.Description, "\t")
	}
	fmt.Fprintf(b, "\t%s %s `%s`\n", field, typ, tags)
	return nil
}

// defaultTag renders a default as chanx's default tag reads it: strings bare,
// anything else as JSON.
func defaultTag(s *Schema, typ string) (string, bool) {
	if len(s.Default) == 0 || string(s.Default) == "null" {
		return "", false
	}
	var str string
	if typ != "any" && json.Unmarshal(s.Default, &str) == nil {
		return str, !strings.Contains(str, "`")
	}
	var compact bytes.Buffer
	if json.Compact(&compact, s.Default) != nil || strings.Contains(compact.String(), "`") {
		return "", false
	}
	return compact.String(), true
}

func (g *generator) goType(s *Schema, owner, field string) (string, error) {
	switch {
	case s.Ref != "":
		name := s.Ref[strings.LastIndex(s.Ref, "/")+1:]
		if _, ok := g.doc.Components.Schemas[name]; !ok {
			return "", fmt.Errorf("codegen: %s.%s: unknown $ref %s", owner, field, s.Ref)
		}
		return goName(name), nil
	case len(s.AnyOf) > 0 || len(s.OneOf) > 0:
		members := append(slices.Clone(s.AnyOf), s.OneOf...)
		var types []string
		nullable := false
		for _, m := range members {
			if isNull(m) {
				nullable = true
				continue
			}
			t, err := g.goType(m, owner, field)
			if err != nil {
				return "", err
			}
			if !slices.Contains(types, t) {
				types = append(types, t)
			}
		}
		if len(types) != 1 {
			return "any", nil
		}
		if nullable && !nilable(types[0]) {
			return "*" + types[0], nil
		}
		return types[0], nil
	case len(s.AllOf) == 1:
		return g.goType(s.AllOf[0], owner, field)
	case len(s.Enum) > 0 || len(s.Const) > 0:
		if len(s.Type) == 1 {
			return scalar(s.Type[0]), nil
		}
		return "any", nil
	case len(s.Type) > 1:
		var types []string
		nullable := false
		for _, t := range s.Type {
			if t == "null" {
				nullable = true
			} else {
				types = append(types, t)
			}
		}
		if len(types) != 1 {
			return "any", nil
		}
		t, err := g.goType(&Schema{Type: typeList{types[0]}, Items: s.Items, Properties: s.Properties, AdditionalProperties: s.AdditionalProperties}, owner, field)
		if err != nil || !nullable || nilable(t) {
			return t, err
		}
		return "*" + t, nil
	case len(s.Type) == 0:
		if len(s.Properties.keys) > 0 {
			return g.lift(s, owner, field), nil
		}
		return "any", nil
	}
	switch s.Type[0] {
	case "array":
		if s.Items == nil {
			return "[]any", nil
		}
		t, err := g.goType(s.Items, owner, field)
		return "[]" + t, err
	case "object":
		if len(s.Properties.keys) > 0 {
			return g.lift(s, owner, field), nil
		}
		if s.AdditionalProperties != nil {
			t, err := g.goType(s.AdditionalProperties, owner, field)
			return "map[string]" + t, err
		}
		return "map[string]any", nil
	}
	return scalar(s.Type[0]), nil
}

// lift names an inline object after its owner and field.
func (g *generator) lift(s *Schema, owner, field string) string {
	name := owner + field
	for i := 2; g.names[name] != ""; i++ {
		name = owner + field + strconv.Itoa(i)
	}
	g.names[name] = "inline object of " + owner
	g.extra = append(g.extra, namedType{name, s})
	return name
}

func scalar(t string) string {
	switch t {
	case "string":
		return "string"
	case "integer":
		return "int"
	case "number":
		return "float64"
	case "boolean":
		return "bool"
	}
	return "any"
}

func isNull(s *Schema) bool { return len(s.Type) == 1 && s.Type[0] == "null" }

func isObject(s *Schema) bool {
	return len(s.Properties.keys) > 0 && (len(s.Type) == 0 || s.Type[0] == "object")
}

func nilable(t string) bool {
	return t == "any" || strings.HasPrefix(t, "[]") || strings.HasPrefix(t, "map[") || strings.HasPrefix(t, "*")
}

func appendNew(list []string, s string) []string {
	if s == "" || slices.Contains(list, s) {
		return list
	}
	return append(list, s)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
