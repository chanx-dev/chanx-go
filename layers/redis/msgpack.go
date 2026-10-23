package redis

import (
	"bytes"

	"github.com/vmihailenco/msgpack/v5"

	"github.com/huynguyengl99/chanx-go/layers/serializer"
)

// MsgPack is the default format: text as str and bytes as bin, so string and
// []byte stay apart. Decoded ints are normalised to int64 by the serializer.
var MsgPack serializer.Format = msgpackFormat{}

type msgpackFormat struct{}

func (msgpackFormat) Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	enc.UseCompactInts(true)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (msgpackFormat) Unmarshal(data []byte) (any, error) {
	// Not the loose decoding: it turns bin into string.
	return msgpack.NewDecoder(bytes.NewReader(data)).DecodeInterface()
}
