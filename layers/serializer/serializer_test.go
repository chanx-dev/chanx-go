package serializer

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"go.chanx.dev/layers"
	"go.chanx.dev/layers/internal/vectors"
)

func TestJSONVectors(t *testing.T) {
	var cases []struct {
		JSON  string `json:"json"`
		Value any    `json:"value"`
	}
	if err := vectors.Load("../testdata/json.json", &cases); err != nil {
		t.Fatal(err)
	}
	s := New(Options{})
	for _, c := range cases {
		want := vectors.Value(c.Value)
		got, err := s.Deserialize([]byte(c.JSON))
		if err != nil {
			t.Fatal(err)
		}
		if d := vectors.Diff("", got, want); d != "" {
			t.Errorf("decoding Python JSON: %s", d)
		}
		// What Go writes decodes to the same values.
		data, err := s.Serialize(want.(map[string]any))
		if err != nil {
			t.Fatal(err)
		}
		again, err := s.Deserialize(data)
		if err != nil {
			t.Fatal(err)
		}
		if d := vectors.Diff("", again, want); d != "" {
			t.Errorf("round trip of %s: %s", data, d)
		}
	}
}

func TestPyFloat(t *testing.T) {
	data, _ := JSON.Marshal(map[string]any{"a": 2.0, "b": 1.5, "c": []any{3.0, 1e16, -1.5e20}, "d": int64(4)})
	if string(data) != `{"a":2.0,"b":1.5,"c":[3.0,1e+16,-1.5e+20],"d":4}` { // as Python's json.dumps
		t.Fatalf("got %s", data)
	}
}

func TestFernetKeyDerivation(t *testing.T) {
	var cases []struct{ Key, Fernet string }
	if err := vectors.Load("../testdata/fernet_keys.json", &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		k := deriveKey(c.Key)
		if got := hex.EncodeToString(k.sign) + hex.EncodeToString(k.enc); got != c.Fernet {
			t.Errorf("key %q: got %s, want %s", c.Key, got, c.Fernet)
		}
	}
}

func TestFernetVectors(t *testing.T) {
	var v struct {
		Fixed []struct {
			Key, Plaintext, IV, Token string
			Time                      int64
		}
		Layer, Rotated []struct {
			Keys  []string
			Token string
			Value any
		}
		MsgpackLayer []struct {
			Keys  []string
			Token string
			Value any
		} `json:"msgpack_layer"`
	}
	if err := vectors.Load("../testdata/fernet.json", &v); err != nil {
		t.Fatal(err)
	}
	for _, c := range v.Fixed {
		plain, _ := base64.StdEncoding.DecodeString(c.Plaintext)
		iv, _ := base64.StdEncoding.DecodeString(c.IV)
		got, err := fernetEncryptIV(deriveKey(c.Key), plain, time.Unix(c.Time, 0), iv)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != c.Token {
			t.Errorf("token differs from Python's:\n got %s\nwant %s", got, c.Token)
		}
		out, err := fernetDecrypt([]fernetKey{deriveKey(c.Key)}, []byte(c.Token), 0, time.Unix(c.Time, 0))
		if err != nil || string(out) != string(plain) {
			t.Errorf("decrypt: %q %v", out, err)
		}
	}
	for _, group := range [][]struct {
		Keys  []string
		Token string
		Value any
	}{v.Layer, v.Rotated} {
		for _, c := range group {
			s := New(Options{SymmetricEncryptionKeys: c.Keys})
			got, err := s.Deserialize([]byte(c.Token))
			if err != nil {
				t.Fatal(err)
			}
			if d := vectors.Diff("", got, vectors.Value(c.Value)); d != "" {
				t.Error(d)
			}
		}
	}
}

func TestFernetRejects(t *testing.T) {
	s := New(Options{SymmetricEncryptionKeys: []string{"a"}})
	data, err := s.Serialize(layers.Message{"type": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(Options{SymmetricEncryptionKeys: []string{"b"}}).Deserialize(data); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("wrong key: %v", err)
	}
	tampered := append([]byte{}, data...)
	tampered[len(tampered)/2] ^= 1
	if _, err := s.Deserialize(tampered); err == nil {
		t.Fatal("tampered token accepted")
	}
	// TTL: Expiry + 10s, as the queue layer.
	k := deriveKey("a")
	old, _ := fernetEncrypt(k, []byte(`{}`), time.Now().Add(-time.Minute))
	if _, err := fernetDecrypt([]fernetKey{k}, old, 30*time.Second, time.Now()); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expired token: %v", err)
	}
	if _, err := fernetDecrypt([]fernetKey{k}, old, 0, time.Now()); err != nil {
		t.Fatalf("no ttl: %v", err)
	}
}

func TestRandomPrefix(t *testing.T) {
	s := New(Options{RandomPrefixLength: 12, SymmetricEncryptionKeys: []string{"k"}})
	a, _ := s.Serialize(layers.Message{"type": "same"})
	b, _ := s.Serialize(layers.Message{"type": "same"})
	if string(a[:12]) == string(b[:12]) {
		t.Fatal("prefixes collide")
	}
	got, err := s.Deserialize(a)
	if err != nil || got["type"] != "same" {
		t.Fatalf("%v %v", got, err)
	}
}
