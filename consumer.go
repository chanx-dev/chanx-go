package chanx

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	"go.chanx.dev/v2/protocol"
)

// Operation documents one handler in AsyncAPI. A struct consumer returns them
// from an Operations() method keyed by method name.
type Operation struct {
	// Name overrides the operation name (the method name in snake case).
	Name        string
	Summary     string
	Description string
	Tags        []string
	// Replies lists what the handler sends when its return type does not say,
	// such as a handler that only broadcasts.
	Replies []Message
}

type handler struct {
	method      string // Go method name, or "" for a function
	op          Operation
	input       *MessageInfo
	outputs     []*MessageInfo
	event       bool
	passthrough bool
	// emitsReply: returns chanx.Message with no Replies, so it documents Emits.
	emitsReply bool
	// ctx is *Context or *EventContext.
	call func(ctx any, msg reflect.Value) (Message, error)
}

// consumerDef is the frozen definition dispatch and AsyncAPI read.
type consumerDef struct {
	typeName string // Go type name, or the name given to NewConsumer
	value    any

	handlers map[string]*handler // client to server, by action
	events   map[string]*handler // server to server, by action
	order    []*handler          // message handlers then events, in documentation order
	emits    []*MessageInfo
	topics   []*topicDef

	authenticate func(*Context, *Request) (any, error)
	authData     func(*Context, any) any
	credentials  []Credential
	groups       []func(*Conn) []string
	onConnect    func(*Context) error
	onDisconnect func(*Context)
	onValidation func(*Context, []Issue)
	onHandlerErr func(*Context, string, error)
	onEventErr   func(*EventContext, string, error)

	// topics only
	authorize     func(*Context, Params) (bool, error)
	onSubscribe   func(*Context) error
	onUnsubscribe func(*Context) error
}

var (
	contextType      = reflect.TypeFor[*Context]()
	eventContextType = reflect.TypeFor[*EventContext]()
	errorType        = reflect.TypeFor[error]()
)

// Hook methods are matched by name and signature before handler discovery.
var hookSignatures = map[string]reflect.Type{
	"Authenticate":       reflect.TypeFor[func(*Context, *Request) (any, error)](),
	"AuthenticationData": reflect.TypeFor[func(*Context, any) any](),
	"Credentials":        reflect.TypeFor[func() []Credential](),
	"Groups":             reflect.TypeFor[func(*Conn) []string](),
	"OnConnect":          reflect.TypeFor[func(*Context) error](),
	"OnDisconnect":       reflect.TypeFor[func(*Context)](),
	"Passthrough":        reflect.TypeFor[func() []Message](),
	"Emits":              reflect.TypeFor[func() []Message](),
	"Topics":             reflect.TypeFor[func() []TopicHandler](),
	"Operations":         reflect.TypeFor[func() map[string]Operation](),
	"OnValidationError":  reflect.TypeFor[func(*Context, []Issue)](),
	"OnHandlerError":     reflect.TypeFor[func(*Context, string, error)](),
	"OnEventError":       reflect.TypeFor[func(*EventContext, string, error)](),
	"Authorize":          reflect.TypeFor[func(*Context, Params) (bool, error)](),
	"OnSubscribe":        reflect.TypeFor[func(*Context) error](),
	"OnUnsubscribe":      reflect.TypeFor[func(*Context) error](),
}

func typeName(v any) string {
	t := reflect.TypeOf(v)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Name()
}

// buildAny builds a struct consumer, a *Consumer, a topic or a TopicConsumer.
func buildAny(v any) (*consumerDef, error) {
	switch x := v.(type) {
	case *Consumer:
		return x.build()
	case *topicConsumer:
		td, err := buildTopic(x.topic)
		if err != nil {
			return nil, err
		}
		return &consumerDef{typeName: td.typeName, value: x, handlers: map[string]*handler{}, events: map[string]*handler{}, topics: []*topicDef{td}}, nil
	case TopicHandler:
		td, err := buildTopic(x)
		if err != nil {
			return nil, err
		}
		return td.consumerDef, nil
	}
	return buildStruct(v)
}

func buildStruct(consumer any) (*consumerDef, error) {
	if consumer == nil {
		return nil, fmt.Errorf("chanx: nil consumer")
	}
	v := reflect.ValueOf(consumer)
	t := v.Type()
	def := &consumerDef{
		typeName: typeName(consumer),
		value:    consumer,
		handlers: map[string]*handler{},
		events:   map[string]*handler{},
	}

	if err := def.bindHooks(v); err != nil {
		return nil, err
	}
	var ops map[string]Operation
	if o, ok := consumer.(interface{ Operations() map[string]Operation }); ok {
		ops = o.Operations()
	}
	var emits []*MessageInfo
	if e, ok := consumer.(interface{ Emits() []Message }); ok {
		for _, m := range e.Emits() {
			info, err := Info(m)
			if err != nil {
				return nil, fmt.Errorf("chanx: %s.Emits: %w", t, err)
			}
			emits = append(emits, info)
		}
	}
	def.emits = emits

	// Methods come sorted by name: the documented operation order.
	var msgHandlers, evtHandlers []*handler
	for i := range t.NumMethod() {
		m := t.Method(i)
		if _, hook := hookSignatures[m.Name]; hook {
			continue // checked in bindHooks
		}
		mt := m.Type // In(0) is the receiver
		if mt.NumIn() < 2 {
			continue
		}
		first := mt.In(1)
		if first != contextType && first != eventContextType {
			continue
		}
		h, err := methodHandler(t, m.Name, mt, v.Method(i), ops[m.Name], emits)
		if err != nil {
			return nil, err
		}
		target, kind := def.handlers, "message"
		if h.event {
			target, kind = def.events, "event"
		}
		if prev, dup := target[h.input.Action]; dup {
			names := []string{prev.method, h.method}
			sort.Strings(names)
			return nil, fmt.Errorf("chanx: %s has two %s handlers for action %q: %s() and %s(). Actions must be unique within a consumer",
				t, kind, h.input.Action, names[0], names[1])
		}
		target[h.input.Action] = h
		if h.event {
			evtHandlers = append(evtHandlers, h)
		} else {
			msgHandlers = append(msgHandlers, h)
		}
	}
	if err := checkPointerMethods(t); err != nil {
		return nil, err
	}
	for name := range ops {
		if !slices.ContainsFunc(append(msgHandlers, evtHandlers...), func(h *handler) bool { return h.method == name }) {
			return nil, fmt.Errorf("chanx: %s.Operations names %q, which is not a handler method", t, name)
		}
	}

	pts, err := collectPassthrough(v)
	if err != nil {
		return nil, err
	}
	for _, info := range pts {
		if _, explicit := def.events[info.Action]; explicit { // an explicit handler wins
			continue
		}
		h := passthroughHandler(info)
		def.events[info.Action] = h
		evtHandlers = append(evtHandlers, h)
	}
	def.order = append(msgHandlers, evtHandlers...)

	if tp, ok := consumer.(interface{ Topics() []TopicHandler }); ok {
		if def.topics, err = buildTopics(t.String(), tp.Topics()); err != nil {
			return nil, err
		}
	}
	return def, nil
}

// checkPointerMethods refuses a consumer passed by value whose handlers or hooks
// have pointer receivers: reflection on the value cannot see them, so the route
// would mount without them.
func checkPointerMethods(t reflect.Type) error {
	if t.Kind() == reflect.Pointer {
		return nil
	}
	pt := reflect.PointerTo(t)
	for i := range pt.NumMethod() {
		m := pt.Method(i)
		if _, ok := t.MethodByName(m.Name); ok {
			continue
		}
		_, hook := hookSignatures[m.Name]
		mt := m.Type
		if hook || (mt.NumIn() >= 2 && (mt.In(1) == contextType || mt.In(1) == eventContextType)) {
			return fmt.Errorf("chanx: %s.%s has a pointer receiver, so a %s value does not have it; pass &%s{} instead", t, m.Name, t, t.Name())
		}
	}
	return nil
}

func buildTopics(owner string, ths []TopicHandler) ([]*topicDef, error) {
	var out []*topicDef
	seen := map[string]bool{}
	for _, th := range ths {
		td, err := buildTopic(th)
		if err != nil {
			return nil, err
		}
		if seen[td.pattern.String()] {
			return nil, fmt.Errorf("chanx: %s mounts topic pattern %q twice", owner, td.pattern)
		}
		seen[td.pattern.String()] = true
		out = append(out, td)
	}
	return out, nil
}

func passthroughHandler(info *MessageInfo) *handler {
	return &handler{
		method:      "",
		op:          Operation{Name: "handle_passthrough_" + protocol.Decamelize(info.Type.Name()), Description: "Passthrough handler for " + info.Type.Name()},
		input:       info,
		outputs:     []*MessageInfo{info},
		event:       true,
		passthrough: true,
	}
}

// collectPassthrough merges Passthrough() of the consumer and of every embedded
// struct, outermost first, without duplicates.
func collectPassthrough(v reflect.Value) ([]*MessageInfo, error) {
	var out []*MessageInfo
	seen := map[reflect.Type]bool{}
	err := eachEmbedded(v, func(x any) error {
		p, ok := x.(interface{ Passthrough() []Message })
		if !ok {
			return nil
		}
		for _, m := range p.Passthrough() {
			info, err := Info(m)
			if err != nil {
				return fmt.Errorf("chanx: Passthrough of %T: %w", x, err)
			}
			if !seen[info.Type] {
				seen[info.Type] = true
				out = append(out, info)
			}
		}
		return nil
	})
	return out, err
}

// eachEmbedded calls fn with the value and then each embedded struct (as a
// pointer when addressable), depth first.
func eachEmbedded(v reflect.Value, fn func(any) error) error {
	if err := fn(v.Interface()); err != nil {
		return err
	}
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	for i := range v.NumField() {
		f := v.Type().Field(i)
		if !f.Anonymous || f.Type == baseTopicType {
			continue
		}
		fv := v.Field(i)
		if fv.Kind() == reflect.Pointer && fv.IsNil() {
			continue
		}
		if fv.Kind() != reflect.Pointer && fv.CanAddr() {
			fv = fv.Addr()
		}
		if !fv.CanInterface() {
			continue
		}
		if err := eachEmbedded(fv, fn); err != nil {
			return err
		}
	}
	return nil
}

func (def *consumerDef) bindHooks(v reflect.Value) error {
	t := v.Type()
	for name, want := range hookSignatures {
		m, ok := t.MethodByName(name)
		if !ok {
			continue
		}
		if got := v.Method(m.Index).Type(); got != want {
			return fmt.Errorf("chanx: %s.%s is a reserved hook name; want %s, got %s", t, name, strings.TrimPrefix(want.String(), "func"), strings.TrimPrefix(got.String(), "func"))
		}
	}
	c := v.Interface()
	if h, ok := c.(interface {
		Authenticate(*Context, *Request) (any, error)
	}); ok {
		def.authenticate = h.Authenticate
	}
	if h, ok := c.(interface{ AuthenticationData(*Context, any) any }); ok {
		def.authData = h.AuthenticationData
	}
	if h, ok := c.(interface{ Credentials() []Credential }); ok {
		def.credentials = h.Credentials()
	}
	if h, ok := c.(interface{ OnConnect(*Context) error }); ok {
		def.onConnect = h.OnConnect
	}
	if h, ok := c.(interface{ OnDisconnect(*Context) }); ok {
		def.onDisconnect = h.OnDisconnect
	}
	if h, ok := c.(interface{ OnValidationError(*Context, []Issue) }); ok {
		def.onValidation = h.OnValidationError
	}
	if h, ok := c.(interface{ OnHandlerError(*Context, string, error) }); ok {
		def.onHandlerErr = h.OnHandlerError
	}
	if h, ok := c.(interface {
		OnEventError(*EventContext, string, error)
	}); ok {
		def.onEventErr = h.OnEventError
	}
	if h, ok := c.(interface {
		Authorize(*Context, Params) (bool, error)
	}); ok {
		def.authorize = h.Authorize
	}
	if h, ok := c.(interface{ OnSubscribe(*Context) error }); ok {
		def.onSubscribe = h.OnSubscribe
	}
	if h, ok := c.(interface{ OnUnsubscribe(*Context) error }); ok {
		def.onUnsubscribe = h.OnUnsubscribe
	}
	// Groups of embedded mixins are merged with the consumer's own; a promoted
	// method is called twice and its groups deduplicated when joining.
	return eachEmbedded(v, func(x any) error {
		if g, ok := x.(interface{ Groups(*Conn) []string }); ok {
			def.groups = append(def.groups, g.Groups)
		}
		return nil
	})
}

const handlerForm = "want func(*chanx.Context | *chanx.EventContext, SomeMessage) error or (Reply, error), where Reply is a message type or chanx.Message"

func methodHandler(t reflect.Type, name string, mt reflect.Type, fn reflect.Value, op Operation, emits []*MessageInfo) (*handler, error) {
	if mt.NumIn() != 3 {
		return nil, fmt.Errorf("chanx: %s.%s: %s", t, name, handlerForm)
	}
	h, err := newHandler(mt.In(1), mt.In(2), outTypes(mt), op, emits)
	if err != nil {
		return nil, fmt.Errorf("chanx: %s.%s: %w", t, name, err)
	}
	h.method = name
	if h.op.Name == "" {
		h.op.Name = protocol.Depascalize(name)
	}
	h.call = reflectCall(fn, len(outTypes(mt)) == 2)
	return h, nil
}

func outTypes(ft reflect.Type) []reflect.Type {
	var out []reflect.Type
	for i := range ft.NumOut() {
		out = append(out, ft.Out(i))
	}
	return out
}

// newHandler checks a handler's types and works out what it documents.
func newHandler(ctxT, in reflect.Type, outs []reflect.Type, op Operation, emits []*MessageInfo) (*handler, error) {
	if ctxT != contextType && ctxT != eventContextType {
		return nil, fmt.Errorf("first parameter must be *chanx.Context or *chanx.EventContext; %s", handlerForm)
	}
	if in.Kind() == reflect.Pointer || !in.Implements(messageType) {
		return nil, fmt.Errorf("second parameter must be a message type (a struct embedding chanx.BaseMessage, not a pointer); %s", handlerForm)
	}
	input, err := infoOf(in)
	if err != nil {
		return nil, err
	}
	h := &handler{op: op, input: input, event: ctxT == eventContextType}
	switch len(outs) {
	case 1:
		if outs[0] != errorType {
			return nil, fmt.Errorf("%s", handlerForm)
		}
	case 2:
		if outs[1] != errorType || !outs[0].Implements(messageType) {
			return nil, fmt.Errorf("%s", handlerForm)
		}
		if outs[0] != messageType {
			if outs[0].Kind() == reflect.Pointer {
				return nil, fmt.Errorf("reply must be a message type, not a pointer; %s", handlerForm)
			}
			info, err := infoOf(outs[0])
			if err != nil {
				return nil, err
			}
			h.outputs = []*MessageInfo{info}
		}
	default:
		return nil, fmt.Errorf("%s", handlerForm)
	}
	if len(op.Replies) > 0 {
		h.outputs = nil
		for _, m := range op.Replies {
			info, err := Info(m)
			if err != nil {
				return nil, err
			}
			h.outputs = append(h.outputs, info)
		}
	} else if len(outs) == 2 && outs[0] == messageType {
		h.emitsReply = true
		h.outputs = slices.Clone(emits)
	}
	return h, nil
}

func reflectCall(fn reflect.Value, hasReply bool) func(any, reflect.Value) (Message, error) {
	return func(ctx any, msg reflect.Value) (Message, error) {
		out := fn.Call([]reflect.Value{reflect.ValueOf(ctx), msg})
		var err error
		if e := out[len(out)-1]; !e.IsNil() {
			err = e.Interface().(error)
		}
		if !hasReply {
			return nil, err
		}
		r := out[0]
		if (r.Kind() == reflect.Interface || r.Kind() == reflect.Pointer) && r.IsNil() {
			return nil, err
		}
		return r.Interface().(Message), err
	}
}

// Consumer is a consumer built from functions instead of methods: for closures,
// dynamic setups and tests. Register handlers with On, Handle and OnEvent; set
// hooks through the fields.
//
//	hub := chanx.NewConsumer("topic_hub")
//	chanx.On(hub, func(ctx *chanx.Context, _ chanx.PingMessage) (chanx.PongMessage, error) {
//		return chanx.PongMessage{}, nil
//	})
type Consumer struct {
	// Name is the consumer's name: the AsyncAPI channel name and the prefix of
	// duplicated operation names.
	Name string

	Authenticate       func(*Context, *Request) (any, error)
	AuthenticationData func(*Context, any) any
	Credentials        []Credential
	Groups             func(*Conn) []string
	OnConnect          func(*Context) error
	OnDisconnect       func(*Context)
	OnValidationError  func(*Context, []Issue)
	OnHandlerError     func(*Context, string, error)
	OnEventError       func(*EventContext, string, error)
	Passthrough        []Message
	Emits              []Message
	Topics             []TopicHandler

	handlers []*handler
	err      error
}

// NewConsumer returns an empty consumer.
func NewConsumer(name string) *Consumer { return &Consumer{Name: name} }

// HandlerOption documents a handler registered with On, Handle or OnEvent.
type HandlerOption func(*Operation)

// Summary sets the operation summary.
func Summary(s string) HandlerOption { return func(o *Operation) { o.Summary = s } }

// OperationDescription sets the operation description.
func OperationDescription(s string) HandlerOption { return func(o *Operation) { o.Description = s } }

// OperationTags sets the operation tags.
func OperationTags(tags ...string) HandlerOption { return func(o *Operation) { o.Tags = tags } }

// OperationName overrides the operation name (the action by default).
func OperationName(name string) HandlerOption { return func(o *Operation) { o.Name = name } }

// Replies documents what a handler sends when its signature does not say.
func Replies(msgs ...Message) HandlerOption { return func(o *Operation) { o.Replies = msgs } }

func (c *Consumer) add(ctxT, in reflect.Type, outs []reflect.Type, opts []HandlerOption, call func(any, reflect.Value) (Message, error)) {
	var op Operation
	for _, o := range opts {
		o(&op)
	}
	// Emits is read in build: it may be set after the handlers.
	h, err := newHandler(ctxT, in, outs, op, nil)
	if err != nil {
		if c.err == nil {
			c.err = fmt.Errorf("chanx: %s: %w", c.Name, err)
		}
		return
	}
	if h.op.Name == "" {
		h.op.Name = h.input.Action
	}
	h.method = h.op.Name
	h.call = call
	c.handlers = append(c.handlers, h)
}

// On registers a handler replying with Out (client to server).
func On[In, Out Message](c *Consumer, fn func(*Context, In) (Out, error), opts ...HandlerOption) {
	c.add(contextType, reflect.TypeFor[In](), []reflect.Type{reflect.TypeFor[Out](), errorType}, opts,
		func(ctx any, msg reflect.Value) (Message, error) {
			out, err := fn(ctx.(*Context), msg.Interface().(In))
			if isNilMessage(out) {
				return nil, err
			}
			return out, err
		})
}

// Handle registers a handler that sends no direct reply.
func Handle[In Message](c *Consumer, fn func(*Context, In) error, opts ...HandlerOption) {
	c.add(contextType, reflect.TypeFor[In](), []reflect.Type{errorType}, opts,
		func(ctx any, msg reflect.Value) (Message, error) {
			return nil, fn(ctx.(*Context), msg.Interface().(In))
		})
}

// OnEvent registers a server to server event handler whose result is sent to
// the client.
func OnEvent[In, Out Message](c *Consumer, fn func(*EventContext, In) (Out, error), opts ...HandlerOption) {
	c.add(eventContextType, reflect.TypeFor[In](), []reflect.Type{reflect.TypeFor[Out](), errorType}, opts,
		func(ctx any, msg reflect.Value) (Message, error) {
			out, err := fn(ctx.(*EventContext), msg.Interface().(In))
			if isNilMessage(out) {
				return nil, err
			}
			return out, err
		})
}

// HandleEvent registers an event handler that sends nothing by itself.
func HandleEvent[In Message](c *Consumer, fn func(*EventContext, In) error, opts ...HandlerOption) {
	c.add(eventContextType, reflect.TypeFor[In](), []reflect.Type{errorType}, opts,
		func(ctx any, msg reflect.Value) (Message, error) {
			return nil, fn(ctx.(*EventContext), msg.Interface().(In))
		})
}

func isNilMessage(m Message) bool {
	if m == nil {
		return true
	}
	v := reflect.ValueOf(m)
	return v.Kind() == reflect.Pointer && v.IsNil()
}

func (c *Consumer) build() (*consumerDef, error) {
	if c.err != nil {
		return nil, c.err
	}
	if c.Name == "" {
		return nil, fmt.Errorf("chanx: NewConsumer needs a name")
	}
	def := &consumerDef{
		typeName:     c.Name,
		value:        c,
		handlers:     map[string]*handler{},
		events:       map[string]*handler{},
		authenticate: c.Authenticate,
		authData:     c.AuthenticationData,
		credentials:  c.Credentials,
		onConnect:    c.OnConnect,
		onDisconnect: c.OnDisconnect,
		onValidation: c.OnValidationError,
		onHandlerErr: c.OnHandlerError,
		onEventErr:   c.OnEventError,
	}
	if c.Groups != nil {
		def.groups = []func(*Conn) []string{c.Groups}
	}
	for _, m := range c.Emits {
		info, err := Info(m)
		if err != nil {
			return nil, err
		}
		def.emits = append(def.emits, info)
	}
	var msgs, evts []*handler
	for _, h := range c.handlers {
		if h.emitsReply {
			cp := *h
			cp.outputs = slices.Clone(def.emits)
			h = &cp
		}
		target, kind := def.handlers, "message"
		if h.event {
			target, kind = def.events, "event"
		}
		if prev, dup := target[h.input.Action]; dup {
			names := []string{prev.method, h.method}
			sort.Strings(names)
			return nil, fmt.Errorf("chanx: %s has two %s handlers for action %q: %s and %s", c.Name, kind, h.input.Action, names[0], names[1])
		}
		target[h.input.Action] = h
		if h.event {
			evts = append(evts, h)
		} else {
			msgs = append(msgs, h)
		}
	}
	for _, m := range c.Passthrough {
		info, err := Info(m)
		if err != nil {
			return nil, err
		}
		if _, explicit := def.events[info.Action]; !explicit {
			h := passthroughHandler(info)
			def.events[info.Action] = h
			evts = append(evts, h)
		}
	}
	def.order = append(msgs, evts...)
	var err error
	if def.topics, err = buildTopics(c.Name, c.Topics); err != nil {
		return nil, err
	}
	return def, nil
}

func (d *consumerDef) describe() string {
	list := func(m map[string]*handler) string {
		var out []string
		for a, h := range m {
			out = append(out, a+"->"+cmpName(h))
		}
		sort.Strings(out)
		return strings.Join(out, ", ")
	}
	s := fmt.Sprintf("messages [%s] events [%s]", list(d.handlers), list(d.events))
	for _, t := range d.topics {
		s += fmt.Sprintf(" topic %s {%s}", t.pattern, t.describe())
	}
	return s
}

func cmpName(h *handler) string {
	if h.passthrough {
		return "passthrough"
	}
	return h.method
}
