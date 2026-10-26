package chanx

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.chanx.dev/internal/schema"
	"go.chanx.dev/layers"
	"go.chanx.dev/protocol"
	"go.chanx.dev/transport"
)

// Issue is one validation problem, as the protocol's error frames list them:
// {"type": "missing", "loc": ["payload", "message"], "msg": "Field required"}.
type Issue = schema.Issue

// prepare decodes a frame and its envelope on the reader goroutine and returns
// the work to run for it, or nil when an error frame was already answered.
func (c *Conn) prepare(typ transport.MessageType, data []byte) func() {
	base := c.baseContext()
	if typ == transport.Binary {
		c.rejectFrame(base, "binary frames are not supported")
		return nil
	}
	if !json.Valid(data) {
		var v any
		err := json.Unmarshal(data, &v)
		reason := "invalid JSON"
		if err != nil {
			reason = err.Error()
		}
		c.rejectFrame(base, reason)
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	_ = dec.Decode(&v)
	obj, ok := v.(map[string]any)
	if !ok {
		c.validationError(base, c.def, []Issue{{Type: "model_type", Loc: []any{}, Msg: "Input should be a valid dictionary or instance of Envelope"}})
		return nil
	}

	var issues []Issue
	if raw, ok := obj[protocol.FieldVersion]; ok && raw != nil {
		if _, is := laxInt(raw, protocol.FieldVersion); is != nil {
			issues = append(issues, *is)
		}
	}
	topic, ref := "", ""
	hasTopic, hasRef := false, false
	if raw, ok := obj[protocol.FieldTopic]; ok && raw != nil {
		s, isStr := raw.(string)
		if !isStr {
			issues = append(issues, Issue{Type: "string_type", Loc: []any{protocol.FieldTopic}, Msg: "Input should be a valid string"})
		}
		topic, hasTopic = s, isStr
	}
	if raw, ok := obj[protocol.FieldRef]; ok && raw != nil {
		s, isStr := raw.(string)
		if !isStr {
			issues = append(issues, Issue{Type: "string_type", Loc: []any{protocol.FieldRef}, Msg: "Input should be a valid string"})
		}
		ref, hasRef = s, isStr
	}
	if raw, ok := obj[protocol.FieldSeq]; ok && raw != nil {
		if _, is := laxInt(raw, protocol.FieldSeq); is != nil {
			issues = append(issues, *is)
		}
	}
	if len(issues) > 0 {
		c.validationError(base, c.def, issues)
		return nil
	}
	content := make(map[string]any, len(obj))
	for k, val := range obj {
		if !protocol.IsEnvelopeField(k) {
			content[k] = val
		}
	}
	if c.route.camelize() {
		content = protocol.DecamelizeKeys(content).(map[string]any)
	}

	if !hasTopic {
		c.mu.Lock()
		topic = c.defaultTopic // a dedicated topic route needs no envelope
		c.mu.Unlock()
		hasTopic = topic != ""
	}

	ctx := &Context{Context: c.ctx, Conn: c, Ref: ref, HasRef: hasRef, logger: c.log}
	action, _ := content[protocol.FieldAction].(string)
	ctx.Action = action
	if c.engine.cfg.LogMessages {
		ctx.logger = c.log.With("message_id", newID(4), "received_action", content[protocol.FieldAction])
		if !c.engine.ignore[action] && !protocol.IsCompleteAction(action) {
			ctx.logger.Info("Websocket received")
		}
	}
	if !hasTopic { // "" is a topic, as in Python: only a missing or null one is not
		return func() {
			defer c.recoverLog("frame")
			c.handleFrame(ctx, c.def, content)
		}
	}
	ctx.Topic, ctx.inTopic = topic, true
	return func() {
		defer c.recoverLog("topic frame")
		switch action {
		case protocol.ActionSubscribe:
			c.subscribe(ctx, topic)
		case protocol.ActionUnsubscribe:
			c.unsubscribe(ctx, topic)
		default:
			c.mu.Lock()
			sub := c.subs[topic]
			c.mu.Unlock()
			if sub == nil {
				c.topicError(ctx, protocol.ReasonNotSubscribed, fmt.Sprintf("Not subscribed to '%s'", topic))
				return
			}
			ctx.Params, ctx.topic = sub.params, sub.def
			c.handleFrame(ctx, sub.def.consumerDef, content)
		}
	}
}

// rejectFrame answers a frame that is not JSON text, keeping the connection.
func (c *Conn) rejectFrame(ctx *Context, reason string) {
	c.log.Warn("Malformed frame", "reason", reason)
	c.validationError(ctx, c.def, []Issue{{Type: "json_invalid", Loc: []any{}, Msg: "Invalid JSON: " + reason}})
}

func (c *Conn) validationError(ctx *Context, def *consumerDef, issues []Issue) {
	defer c.recoverLog("OnValidationError") // also runs on the reader goroutine
	switch {
	case def.onValidation != nil:
		def.onValidation(ctx, issues)
	case c.engine.cfg.OnValidationError != nil:
		c.engine.cfg.OnValidationError(ctx, issues)
	default:
		_ = ctx.Send(ErrorMessage{Payload: issues})
	}
}

func (c *Conn) handlerError(ctx *Context, def *consumerDef, action string, err error) {
	defer c.recoverLog("OnHandlerError")
	switch {
	case def.onHandlerErr != nil:
		def.onHandlerErr(ctx, action, err)
	case c.engine.cfg.OnHandlerError != nil:
		c.engine.cfg.OnHandlerError(ctx, action, err)
	default:
		_ = ctx.Send(ErrorMessage{Payload: errorDetail{Detail: "Failed to process message for " + action}})
		ctx.Logger().Error(fmt.Sprintf("Handler error for action '%s': %v", action, err), "action", action, "err", err)
	}
}

func (c *Conn) eventError(ctx *EventContext, def *consumerDef, action string, err error) {
	defer c.recoverLog("OnEventError")
	switch {
	case def.onEventErr != nil:
		def.onEventErr(ctx, action, err)
	case c.engine.cfg.OnEventError != nil:
		c.engine.cfg.OnEventError(ctx, action, err)
	default:
		ctx.Logger().Error(fmt.Sprintf("Event handler error for action '%s': %v", action, err), "action", action, "err", err)
	}
}

var errNoHandlers = errors.New("no message handlers registered on this consumer")

// handleFrame validates a frame against the handlers, runs the matching one,
// answers errors, and sends completion when configured.
func (c *Conn) handleFrame(ctx *Context, def *consumerDef, content map[string]any) {
	defer func() {
		if c.route.sendCompletion() {
			_ = ctx.Send(CompleteMessage{})
		}
	}()
	h, msg, issues, err := decodeMessage(def.handlers, content)
	if err != nil {
		_ = ctx.Send(ErrorMessage{Payload: errorDetail{Detail: "Failed to process message"}})
		ctx.Logger().Error("Failed to process message: "+err.Error(), "err", err)
		return
	}
	if len(issues) > 0 {
		c.validationError(ctx, def, issues)
		return
	}
	ctx.Action = h.input.Action
	out, err := invoke(h, ctx, msg)
	if err != nil {
		c.handlerError(ctx, def, h.input.Action, err)
		return
	}
	if out != nil {
		_ = ctx.Send(out)
	}
}

// invoke calls a handler, turning a panic into an error.
func invoke(h *handler, ctx any, msg reflect.Value) (out Message, err error) {
	defer func() {
		if p := recover(); p != nil {
			out, err = nil, fmt.Errorf("panic: %s", panicText(p))
		}
	}()
	if h.passthrough {
		return msg.Interface().(Message), nil
	}
	return h.call(ctx, msg)
}

func panicText(p any) string {
	if e, ok := p.(error); ok {
		return e.Error()
	}
	return fmt.Sprint(p)
}

// decodeMessage validates content against a handler set: with one handler the
// content validates as that message (a wrong action is a literal_error), with
// several as a union discriminated by action (the action leads every loc).
func decodeMessage(handlers map[string]*handler, content map[string]any) (*handler, reflect.Value, []Issue, error) {
	if len(handlers) == 0 {
		return nil, reflect.Value{}, nil, errNoHandlers
	}
	rawAction, present := content[protocol.FieldAction]
	var h *handler
	var issues []Issue
	prefix := []any{}
	if len(handlers) == 1 {
		for _, only := range handlers {
			h = only
		}
		if s, ok := rawAction.(string); present && (!ok || s != h.input.Action) {
			issues = append(issues, Issue{Type: "literal_error", Loc: []any{protocol.FieldAction}, Msg: "Input should be " + schema.PyRepr(h.input.Action)})
		}
	} else {
		if !present {
			return nil, reflect.Value{}, []Issue{{Type: "union_tag_not_found", Loc: []any{}, Msg: "Unable to extract tag using discriminator 'action'"}}, nil
		}
		tag := schema.PyRepr(rawAction)
		if s, ok := rawAction.(string); ok {
			tag = s
			h = handlers[s]
		}
		if h == nil {
			return nil, reflect.Value{}, []Issue{{Type: "union_tag_invalid", Loc: []any{}, Msg: fmt.Sprintf(
				"Input tag '%s' found using 'action' does not match any of the expected tags: %s", tag, expectedTags(handlers))}}, nil
		}
		prefix = []any{h.input.Action}
	}
	msg, more := decodePayload(h.input, content, prefix)
	issues = append(issues, more...)
	if len(issues) > 0 {
		return h, reflect.Value{}, issues, nil
	}
	return h, msg, nil, nil
}

func expectedTags(handlers map[string]*handler) string {
	actions := make([]string, 0, len(handlers))
	for a := range handlers {
		actions = append(actions, a)
	}
	slices.Sort(actions)
	for i, a := range actions {
		actions[i] = "'" + a + "'"
	}
	return strings.Join(actions, ", ")
}

func decodePayload(info *MessageInfo, content map[string]any, prefix []any) (reflect.Value, []Issue) {
	msg := reflect.New(info.Type).Elem()
	loc := append(slices.Clone(prefix), protocol.FieldPayload)
	raw, has := content[protocol.FieldPayload]
	if info.PayloadType == nil {
		if has && raw != nil {
			return msg, []Issue{{Type: "none_required", Loc: loc, Msg: "Input should be None"}}
		}
		return msg, nil
	}
	field := msg.FieldByIndex(info.payloadIndex)
	if !has {
		if !info.PayloadOptional {
			return msg, []Issue{{Type: "missing", Loc: loc, Msg: "Field required"}}
		}
		if info.HasDefault {
			_ = json.Unmarshal(schema.DefaultJSON(info.PayloadType, info.PayloadDefault), field.Addr().Interface())
		}
		return msg, nil
	}
	v, err := info.payloadValidator()
	if err != nil {
		return msg, []Issue{{Type: "value_error", Loc: loc, Msg: err.Error()}}
	}
	data, err := json.Marshal(jsonFloats(raw))
	if err != nil {
		return msg, []Issue{{Type: "value_error", Loc: loc, Msg: err.Error()}}
	}
	val, issues := v.Decode(data, loc)
	if len(issues) > 0 {
		return msg, issues
	}
	field.Set(val)
	return msg, nil
}

// laxInt validates an envelope integer leniently: "12" and true are accepted,
// 1.5 is not.
func laxInt(raw any, name string) (int64, *Issue) {
	switch x := raw.(type) {
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i, nil
		}
		if f, err := x.Float64(); err == nil && f == float64(int64(f)) {
			return int64(f), nil
		}
		return 0, &Issue{Type: "int_from_float", Loc: []any{name}, Msg: "Input should be a valid integer, got a number with a fractional part"}
	case string:
		if i, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64); err == nil {
			return i, nil
		}
		return 0, &Issue{Type: "int_parsing", Loc: []any{name}, Msg: "Input should be a valid integer, unable to parse string as an integer"}
	case bool:
		if x {
			return 1, nil
		}
		return 0, nil
	}
	return 0, &Issue{Type: "int_type", Loc: []any{name}, Msg: "Input should be a valid integer"}
}

func (c *Conn) topicError(ctx *Context, reason, detail string) {
	c.log.Warn("Topic error", "topic", ctx.Topic, "reason", reason, "detail", detail)
	_ = ctx.Send(ErrorMessage{Payload: topicError{Reason: reason, Detail: detail}})
}

func (c *Conn) matchTopic(topic string) (*topicDef, Params) {
	for _, td := range c.def.topics {
		if p, ok := td.pattern.Match(topic); ok {
			return td, p
		}
	}
	return nil, nil
}

// subscribe authorizes a topic, joins its group and confirms; a duplicate
// subscribe confirms again without rerunning OnSubscribe.
func (c *Conn) subscribe(ctx *Context, topic string) {
	ctx.Topic = topic
	c.mu.Lock()
	_, already := c.subs[topic]
	c.mu.Unlock()
	if already {
		_ = ctx.Send(SubscribedMessage{})
		return
	}
	td, params := c.matchTopic(topic)
	if td == nil {
		c.topicError(ctx, protocol.ReasonUnknownTopic, fmt.Sprintf("Unknown topic '%s'", topic))
		return
	}
	ctx.Params, ctx.topic = params, td
	if td.authorize != nil {
		var allowed bool
		err := safeCall(func() error {
			var err error
			allowed, err = td.authorize(ctx, params)
			return err
		})
		if err != nil {
			c.log.Error("chanx: Authorize failed", "topic", topic, "err", err)
		}
		if err != nil || !allowed {
			c.topicError(ctx, protocol.ReasonUnauthorized, fmt.Sprintf("Not allowed to subscribe to '%s'", topic))
			return
		}
	}
	group, err := groupName(td.namespace, topic)
	if err == nil {
		err = c.Join(ctx, group)
	}
	if err != nil {
		c.log.Error("chanx: cannot join topic group", "topic", topic, "err", err)
		_ = ctx.Send(ErrorMessage{Payload: errorDetail{Detail: "Failed to process message"}})
		return
	}
	c.mu.Lock()
	if c.torn {
		c.mu.Unlock()
		return
	}
	_, raced := c.subs[topic]
	if !raced {
		c.subs[topic] = &subscription{def: td, params: params}
	}
	c.mu.Unlock()
	_ = ctx.Send(SubscribedMessage{})
	if raced || td.onSubscribe == nil {
		return
	}
	// Anything the topic pushes now is state, not a reply to the request.
	state := &Context{Context: ctx.Context, Conn: c, Topic: topic, Params: params, topic: td, logger: ctx.logger}
	if err := safeCall(func() error { return td.onSubscribe(state) }); err != nil {
		c.log.Error("chanx: OnSubscribe failed", "topic", topic, "err", err)
	}
}

func (c *Conn) unsubscribe(ctx *Context, topic string) {
	c.mu.Lock()
	sub := c.subs[topic]
	delete(c.subs, topic)
	c.mu.Unlock()
	if sub != nil {
		uctx := *ctx
		uctx.Params, uctx.topic = sub.params, sub.def
		c.runUnsubscribeHook(&uctx, sub.def)
		if group, err := groupName(sub.def.namespace, topic); err == nil {
			_ = c.Leave(ctx, group)
		}
	}
	_ = ctx.Send(UnsubscribedMessage{})
}

func (c *Conn) runUnsubscribeHook(ctx *Context, td *topicDef) {
	if td.onUnsubscribe == nil {
		return
	}
	if err := safeCall(func() error { return td.onUnsubscribe(ctx) }); err != nil {
		c.log.Error("chanx: OnUnsubscribe failed", "topic", ctx.Topic, "err", err)
	}
}

// layerLoop dispatches layer messages one at a time, in arrival order, as
// channels' dispatch loop does, so topic events keep their seq order.
func (c *Conn) layerLoop(done chan<- struct{}) {
	defer close(done)
	for {
		msg, err := c.layer.Receive(c.ctx, c.ChannelName)
		if err != nil {
			if c.ctx.Err() != nil || errors.Is(err, layers.ErrClosed) {
				return
			}
			c.log.Error("chanx: layer receive failed", "err", err)
			select {
			case <-c.ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
			continue
		}
		c.dispatchLayer(msg)
	}
}

func (c *Conn) dispatchLayer(msg layers.Message) {
	defer c.recoverLog("layer message")
	base := c.baseContext()
	switch msg["type"] {
	case protocol.EventGroup:
		if exclude, _ := msg["exclude_current"].(bool); exclude && msg["from_channel"] == c.ChannelName {
			return
		}
		data, err := encodeRaw(msg["message"], c.route.camelize())
		if err != nil {
			c.log.Error("chanx: cannot encode group message", "err", err)
			return
		}
		if c.engine.cfg.LogMessages {
			if m, ok := msg["message"].(map[string]any); ok {
				if a, _ := m["action"].(string); !c.engine.ignore[a] {
					c.log.Info("Websocket sent", "sent_action", a)
				}
			}
		}
		_ = c.write(data)
		if c.route.sendCompletion() {
			_ = base.Push(GroupCompleteMessage{})
		}
	case protocol.EventChannel:
		c.handleEvent(base, c.def, msg["event_data"])
	case protocol.EventTopic:
		topic, _ := msg["topic"].(string)
		c.mu.Lock()
		sub := c.subs[topic]
		c.mu.Unlock()
		if sub == nil {
			return
		}
		ctx := &Context{Context: c.ctx, Conn: c, Topic: topic, Params: sub.params, topic: sub.def, logger: c.log}
		if seq, ok := layers.Int(msg["seq"]); ok {
			ctx.Seq = &seq
		}
		c.handleEvent(ctx, sub.def.consumerDef, msg["event_data"])
	default:
		c.log.Warn("chanx: unknown layer message type", "type", msg["type"])
	}
}

// handleEvent validates a layer event, runs its handler and sends the result;
// problems are logged, never sent.
func (c *Conn) handleEvent(ctx *Context, def *consumerDef, data any) {
	if len(def.events) == 0 {
		ctx.Logger().Error("No event handlers registered on this consumer")
		return
	}
	content, ok := data.(map[string]any)
	if !ok {
		ctx.Logger().Error("Channel event validation failed: event_data is not a map")
		return
	}
	h, msg, issues, _ := decodeMessage(def.events, content)
	if len(issues) > 0 {
		ctx.Logger().Error("Channel event validation failed", "issues", issues)
		return
	}
	ctx.Action = h.input.Action
	ec := &EventContext{Context: ctx}
	out, err := invoke(h, ec, msg)
	if err != nil {
		c.eventError(ec, def, h.input.Action, err)
	} else if out != nil {
		_ = ctx.Push(out)
	}
	if c.route.sendCompletion() {
		_ = ctx.Push(EventCompleteMessage{})
	}
}
