package chanx

import (
	"fmt"
	"reflect"

	"github.com/huynguyengl99/chanx-go/protocol"
)

// BaseTopic marks a struct as a topic, with its pattern in the tag:
//
//	type RoomTopic struct {
//		chanx.BaseTopic `pattern:"room:{room_name}" description:"A chat room"`
//	}
//
// Tag keys: pattern (required), name (the group namespace, the type name by
// default), description, and layer (the layer alias
// BroadcastTopic publishes on when the publisher was not pointed at one).
type BaseTopic struct{}

func (BaseTopic) chanxTopic() {}

// TopicHandler is a struct embedding BaseTopic: a pattern plus its own handlers
// and hooks (Authorize, OnSubscribe, OnUnsubscribe, Passthrough).
type TopicHandler interface{ chanxTopic() }

// Params are a topic's or a route's parameters.
type Params map[string]string

var baseTopicType = reflect.TypeFor[BaseTopic]()

type topicDef struct {
	*consumerDef
	pattern     *protocol.Pattern
	namespace   string // group namespace
	description string
	layer       string
}

type topicMeta struct {
	pattern     *protocol.Pattern
	namespace   string
	description string
	layer       string
}

func topicMetaOf(t reflect.Type) (*topicMeta, error) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	base, ok := t.FieldByName("BaseTopic")
	if !ok || base.Type != baseTopicType || len(base.Index) != 1 {
		return nil, fmt.Errorf("chanx: %s must embed chanx.BaseTopic directly", t)
	}
	raw := base.Tag.Get("pattern")
	if raw == "" {
		return nil, fmt.Errorf("chanx: %s embeds chanx.BaseTopic without a `pattern` tag", t)
	}
	p, err := protocol.ParsePattern(raw)
	if err != nil {
		return nil, fmt.Errorf("chanx: %s: %w", t, err)
	}
	m := &topicMeta{pattern: p, namespace: t.Name(), description: base.Tag.Get("description"), layer: base.Tag.Get("layer")}
	if name := base.Tag.Get("name"); name != "" {
		m.namespace = name
	}
	return m, nil
}

func buildTopic(th TopicHandler) (*topicDef, error) {
	meta, err := topicMetaOf(reflect.TypeOf(th))
	if err != nil {
		return nil, err
	}
	def, err := buildStruct(th)
	if err != nil {
		return nil, err
	}
	if _, ok := th.(interface{ Topics() []TopicHandler }); ok {
		return nil, fmt.Errorf("chanx: topic %s cannot host topics", def.typeName)
	}
	return &topicDef{consumerDef: def, pattern: meta.pattern, namespace: meta.namespace, description: meta.description, layer: meta.layer}, nil
}

// groupName is the layer group of a topic: the topic in the type's namespace.
func groupName(namespace, topic string) (string, error) {
	return protocol.SafeGroupName(namespace, topic)
}

type topicConsumer struct{ topic TopicHandler }

// TopicConsumer serves one topic on its own route:
// connecting subscribes the topic filled from the URL parameters, and frames
// need no envelope.
func TopicConsumer(t TopicHandler) any { return &topicConsumer{topic: t} }

// FormatTopic fills topic T's pattern: FormatTopic[RoomTopic](Params{"room_name": "lobby"}).
func FormatTopic[T TopicHandler](p Params) (string, error) {
	meta, err := topicMetaOf(reflect.TypeFor[T]())
	if err != nil {
		return "", err
	}
	return meta.pattern.Format(p)
}

// TopicGroup returns the layer group topic T's subscribers to topic join.
func TopicGroup[T TopicHandler](topic string) (string, error) {
	meta, err := topicMetaOf(reflect.TypeFor[T]())
	if err != nil {
		return "", err
	}
	return groupName(meta.namespace, topic)
}
