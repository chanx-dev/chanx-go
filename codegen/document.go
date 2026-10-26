package codegen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

// Load reads an AsyncAPI document, JSON or YAML, from a file or an http(s) URL.
func Load(ctx context.Context, source string) ([]byte, error) {
	var data []byte
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("codegen: GET %s: %s", source, resp.Status)
		}
		if data, err = io.ReadAll(resp.Body); err != nil {
			return nil, err
		}
	} else {
		var err error
		if data, err = os.ReadFile(source); err != nil {
			return nil, err
		}
	}
	if json.Valid(data) {
		return data, nil
	}
	return yaml.YAMLToJSON(data)
}

type document struct {
	AsyncAPI   string                 `json:"asyncapi"`
	Info       struct{ Title string } `json:"info"`
	Channels   map[string]*channel    `json:"channels"`
	Operations map[string]operation   `json:"operations"`
	Components struct {
		Messages map[string]struct {
			Payload ref `json:"payload"`
		} `json:"messages"`
		Schemas map[string]*Schema `json:"schemas"`
	} `json:"components"`
}

type channel struct {
	Address  string         `json:"address"`
	Title    string         `json:"title"`
	Messages map[string]ref `json:"messages"`
	Topic    *struct {
		Name       string   `json:"name"`
		Pattern    string   `json:"pattern"`
		Parameters []string `json:"parameters"`
	} `json:"x-topic"`
}

type operation struct {
	Action   string `json:"action"`
	Channel  ref    `json:"channel"`
	Messages []ref  `json:"messages"`
	Reply    *struct {
		Messages []ref `json:"messages"`
	} `json:"reply"`
}

type ref struct {
	Ref string `json:"$ref"`
}

func (r ref) name() string { return r.Ref[strings.LastIndex(r.Ref, "/")+1:] }

// Schema is the JSON Schema subset Pydantic writes.
type Schema struct {
	Ref                  string          `json:"$ref"`
	Type                 typeList        `json:"type"`
	Title                string          `json:"title"`
	Description          string          `json:"description"`
	Properties           properties      `json:"properties"`
	Required             []string        `json:"required"`
	Items                *Schema         `json:"items"`
	AdditionalProperties *Schema         `json:"-"`
	AnyOf                []*Schema       `json:"anyOf"`
	OneOf                []*Schema       `json:"oneOf"`
	AllOf                []*Schema       `json:"allOf"`
	Enum                 []any           `json:"enum"`
	Const                json.RawMessage `json:"const"`
	Default              json.RawMessage `json:"default"`
}

func (s *Schema) UnmarshalJSON(data []byte) error {
	type plain Schema
	var aux struct {
		*plain
		Additional json.RawMessage `json:"additionalProperties"`
	}
	aux.plain = (*plain)(s)
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if len(aux.Additional) > 0 && aux.Additional[0] == '{' {
		s.AdditionalProperties = &Schema{}
		return json.Unmarshal(aux.Additional, s.AdditionalProperties)
	}
	return nil
}

type typeList []string

func (t *typeList) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '[' {
		return json.Unmarshal(data, (*[]string)(t))
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*t = typeList{s}
	return nil
}

// properties keeps the document's order: it becomes the struct field order.
type properties struct {
	keys   []string
	values map[string]*Schema
}

func (p *properties) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	if _, err := dec.Token(); err != nil {
		return err
	}
	p.values = map[string]*Schema{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		key := tok.(string)
		s := &Schema{}
		if err := dec.Decode(s); err != nil {
			return err
		}
		p.keys = append(p.keys, key)
		p.values[key] = s
	}
	_, err := dec.Token()
	return err
}
