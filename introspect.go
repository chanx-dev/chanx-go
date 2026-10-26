package chanx

import (
	"strings"

	"go.chanx.dev/v2/protocol"
)

// RouteInfo describes a mounted route for documentation tools.
type RouteInfo struct {
	Path        string
	ParamNames  []string
	Name        string // channel name given with chanx.Name, or ""
	Description string
	Tags        []string
	Credentials []Credential // what the route's Authenticate reads
	Consumer    ConsumerInfo
	// Topics are the topics the consumer hosts; Standalone is set for a
	// TopicConsumer route, which is that topic.
	Topics     []TopicInfo
	Standalone *TopicInfo
}

// ConsumerInfo describes a consumer's handlers.
type ConsumerInfo struct {
	TypeName string
	// SnakeName is the type name without "Consumer", in snake case: the default
	// channel name and the prefix of duplicated operation names.
	SnakeName string
	Handlers  []HandlerInfo
	Events    []HandlerInfo
	Emits     []*MessageInfo
}

// TopicInfo describes a topic.
type TopicInfo struct {
	ConsumerInfo
	Pattern     string
	Params      []string
	Namespace   string
	Description string
}

// HandlerInfo describes one handler.
type HandlerInfo struct {
	Method      string
	Operation   string
	Action      string
	Input       *MessageInfo
	Outputs     []*MessageInfo
	Summary     string
	Description string
	Tags        []string
	Passthrough bool
}

// Info describes the route.
func (r *Route) Info() RouteInfo {
	ri := RouteInfo{
		Path:        r.Path,
		ParamNames:  append([]string(nil), r.ParamNames...),
		Name:        r.opts.name,
		Description: r.opts.description,
		Tags:        append([]string(nil), r.opts.tags...),
		Credentials: append([]Credential(nil), r.def.credentials...),
		Consumer:    consumerInfo(r.def, true),
	}
	for _, td := range r.def.topics {
		ti := topicInfo(td)
		if r.defaultTopic == td {
			ri.Standalone = &ti
		} else {
			ri.Topics = append(ri.Topics, ti)
		}
	}
	return ri
}

func consumerInfo(d *consumerDef, consumer bool) ConsumerInfo {
	name := d.typeName
	if consumer {
		name = strings.TrimSuffix(name, "Consumer")
	}
	ci := ConsumerInfo{TypeName: d.typeName, SnakeName: protocol.Decamelize(name), Emits: d.emits}
	for _, h := range d.order {
		hi := HandlerInfo{
			Method: h.method, Operation: h.op.Name, Action: h.input.Action, Input: h.input,
			Outputs: h.outputs, Summary: h.op.Summary, Description: h.op.Description, Tags: h.op.Tags,
			Passthrough: h.passthrough,
		}
		if h.event {
			ci.Events = append(ci.Events, hi)
		} else {
			ci.Handlers = append(ci.Handlers, hi)
		}
	}
	return ci
}

func topicInfo(td *topicDef) TopicInfo {
	return TopicInfo{
		ConsumerInfo: consumerInfo(td.consumerDef, false),
		Pattern:      td.pattern.String(),
		Params:       td.pattern.Params(),
		Namespace:    td.namespace,
		Description:  td.description,
	}
}

// Declared returns every action the route may send to its clients: replies,
// event outputs, Emits, and the framework's own frames.
func (r *Route) Declared() map[string]bool {
	out := map[string]bool{}
	for _, a := range []string{protocol.ActionError, protocol.ActionComplete, protocol.ActionEventComplete,
		protocol.ActionGroupComplete, protocol.ActionAuthentication, protocol.ActionSubscribed, protocol.ActionUnsubscribed} {
		out[a] = true
	}
	add := func(d *consumerDef) {
		for _, h := range d.order {
			for _, o := range h.outputs {
				out[o.Action] = true
			}
		}
		for _, e := range d.emits {
			out[e.Action] = true
		}
	}
	add(r.def)
	for _, td := range r.def.topics {
		add(td.consumerDef)
	}
	return out
}
