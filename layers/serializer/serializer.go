// Package serializer turns layer messages into bytes: a format (JSON here,
// msgpack in layers/redis), optional Fernet encryption keyed as channels_redis
// keys it, and an optional random prefix.
package serializer

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"go.chanx.dev/layers/v2"
)

// Serializer converts layer messages to bytes and back.
type Serializer interface {
	Serialize(msg layers.Message) ([]byte, error)
	Deserialize(data []byte) (layers.Message, error)
}

// Format is the encoding under encryption: JSON, or msgpack from layers/redis.
type Format interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte) (any, error)
}

// Options configures a serializer.
type Options struct {
	Format Format // default JSON
	// SymmetricEncryptionKeys enables Fernet: the first key encrypts, any key
	// decrypts, so keys can be rotated by prepending a new one.
	SymmetricEncryptionKeys []string
	// RandomPrefixLength prepends random bytes, so identical messages stay distinct
	// members of a sorted set (the queue layer uses 12).
	RandomPrefixLength int
	// Expiry rejects encrypted messages older than Expiry + 10s. Zero means no
	// limit (the pub/sub layer).
	Expiry time.Duration
}

type serializer struct {
	format Format
	keys   []fernetKey
	prefix int
	ttl    time.Duration
}

// New builds a serializer from options.
func New(opts Options) Serializer {
	s := &serializer{format: opts.Format, prefix: opts.RandomPrefixLength}
	if s.format == nil {
		s.format = JSON
	}
	for _, k := range opts.SymmetricEncryptionKeys {
		s.keys = append(s.keys, deriveKey(k))
	}
	if opts.Expiry > 0 {
		s.ttl = opts.Expiry + 10*time.Second
	}
	return s
}

func (s *serializer) Serialize(msg layers.Message) ([]byte, error) {
	data, err := s.format.Marshal(msg)
	if err != nil {
		return nil, err
	}
	if len(s.keys) > 0 {
		if data, err = fernetEncrypt(s.keys[0], data, time.Now()); err != nil {
			return nil, err
		}
	}
	if s.prefix > 0 {
		p := make([]byte, s.prefix, s.prefix+len(data))
		_, _ = rand.Read(p)
		data = append(p, data...)
	}
	return data, nil
}

func (s *serializer) Deserialize(data []byte) (layers.Message, error) {
	if s.prefix > 0 {
		if len(data) < s.prefix {
			return nil, errors.New("serializer: message shorter than its random prefix")
		}
		data = data[s.prefix:]
	}
	if len(s.keys) > 0 {
		var err error
		if data, err = fernetDecrypt(s.keys, data, s.ttl, time.Now()); err != nil {
			return nil, err
		}
	}
	v, err := s.format.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	msg, ok := layers.Normalize(v).(map[string]any)
	if !ok {
		return nil, fmt.Errorf("serializer: message is %T, not a map", v)
	}
	return msg, nil
}

// JSON is UTF-8 JSON, ints and floats kept apart.
var JSON Format = jsonFormat{}

type jsonFormat struct{}

func (jsonFormat) Marshal(v any) ([]byte, error) { return json.Marshal(pyFloats(v)) }

func (jsonFormat) Unmarshal(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber() // keep 7 an int, as json.loads does
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// pyFloat marshals integral floats as Python does (2.0 and 1e+16, where Go
// writes 2 and 10000000000000000), so a receiver still decodes a float.
type pyFloat float64

func (f pyFloat) MarshalJSON() ([]byte, error) {
	v := float64(f)
	switch {
	case v != math.Trunc(v) || math.IsInf(v, 0):
		return json.Marshal(v)
	case math.Abs(v) < 1e16:
		return []byte(strconv.FormatFloat(v, 'f', 1, 64)), nil
	default:
		return []byte(strconv.FormatFloat(v, 'e', -1, 64)), nil
	}
}

func pyFloats(v any) any {
	switch x := v.(type) {
	case float64:
		return pyFloat(x)
	case float32:
		return pyFloat(x)
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = pyFloats(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = pyFloats(val)
		}
		return out
	}
	return v
}
