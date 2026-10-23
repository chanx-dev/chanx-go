package protocol

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func load(t *testing.T, name string, v any) {
	t.Helper()
	data, err := os.ReadFile("../testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatal(err)
	}
}

func TestSafeGroupNameVectors(t *testing.T) {
	var cases []struct {
		Parts     []string
		Namespace *string
		Name      string
	}
	load(t, "safe_group_name.json", &cases)
	for _, c := range cases {
		ns := ""
		if c.Namespace != nil {
			ns = *c.Namespace
		}
		got, err := SafeGroupName(ns, c.Parts...)
		if err != nil || got != c.Name {
			t.Errorf("SafeGroupName(%q, %q) = %q, %v; want %q", ns, c.Parts, got, err, c.Name)
		}
	}
	if _, err := SafeGroupName("", "!!!"); err == nil {
		t.Error("empty name accepted")
	}
}

func TestPatternVectors(t *testing.T) {
	var cases []struct {
		Pattern string
		Params  []string
		Parse   []struct {
			Topic  string
			Params map[string]string
		}
	}
	load(t, "patterns.json", &cases)
	for _, c := range cases {
		p := MustParsePattern(c.Pattern)
		if got := p.Params(); !reflect.DeepEqual(got, c.Params) && (len(got) != 0 || len(c.Params) != 0) {
			t.Errorf("%s: params %v, want %v", c.Pattern, got, c.Params)
		}
		for _, pc := range c.Parse {
			got, ok := p.Match(pc.Topic)
			if ok != (pc.Params != nil) || (ok && !reflect.DeepEqual(got, pc.Params)) {
				t.Errorf("%s.Match(%q) = %v %v, want %v", c.Pattern, pc.Topic, got, ok, pc.Params)
			}
			if ok {
				if back, err := p.Format(got); err != nil || back != pc.Topic {
					t.Errorf("%s.Format(%v) = %q %v", c.Pattern, got, back, err)
				}
			}
		}
	}
	if _, err := MustParsePattern("room:{name}").Format(nil); err == nil {
		t.Error("missing parameter accepted")
	}
	for _, bad := range []string{"", "room:{a}:{a}", "room:{", "room:{a b}"} {
		if _, err := ParsePattern(bad); err == nil {
			t.Errorf("ParsePattern(%q) accepted", bad)
		}
	}
}

func TestHumpsVectors(t *testing.T) {
	var v struct {
		Words []struct {
			In, Camelize, Decamelize, Depascalize string
		}
		Nested []struct {
			In, Camelize, Decamelize any
		}
	}
	load(t, "humps.json", &v)
	for _, w := range v.Words {
		if got := Camelize(w.In); got != w.Camelize {
			t.Errorf("Camelize(%q) = %q, want %q", w.In, got, w.Camelize)
		}
		if got := Decamelize(w.In); got != w.Decamelize {
			t.Errorf("Decamelize(%q) = %q, want %q", w.In, got, w.Decamelize)
		}
		if got := Depascalize(w.In); got != w.Depascalize {
			t.Errorf("Depascalize(%q) = %q, want %q", w.In, got, w.Depascalize)
		}
	}
	for _, n := range v.Nested {
		if got := CamelizeKeys(n.In); !reflect.DeepEqual(got, n.Camelize) {
			t.Errorf("CamelizeKeys(%v) = %v, want %v", n.In, got, n.Camelize)
		}
		if got := DecamelizeKeys(n.In); !reflect.DeepEqual(got, n.Decamelize) {
			t.Errorf("DecamelizeKeys(%v) = %v, want %v", n.In, got, n.Decamelize)
		}
	}
}

func TestPaths(t *testing.T) {
	if got := PathParams("/ws/{a}/x/{b}"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatal(got)
	}
	if got := ColonPath("/ws/{a}/x/{b}"); got != "/ws/:a/x/:b" {
		t.Fatal(got)
	}
	re := PathRegexp("/ws/chat/{room}", true)
	m := re.FindStringSubmatch("/api/ws/chat/lobby")
	if m == nil || m[re.SubexpIndex("room")] != "lobby" {
		t.Fatalf("suffix match: %v", m)
	}
	if PathRegexp("/ws/chat/{room}", false).MatchString("/api/ws/chat/lobby") {
		t.Fatal("anchored regexp matched a prefixed path")
	}
	if PathRegexp("/ws/chat/{room}", false).MatchString("/ws/chat/a/b") {
		t.Fatal("a parameter matched two segments")
	}
	for _, ok := range []string{"/", "/ws", "/ws/{a}/x/{b}"} {
		if err := ValidatePath(ok); err != nil {
			t.Errorf("ValidatePath(%q): %v", ok, err)
		}
	}
	for _, bad := range []string{"ws", "/ws/{rest...}", "/ws/x{a}", "/ws/{a}/{a}", "/ws/{a", "/ws/{a b}"} {
		if err := ValidatePath(bad); err == nil {
			t.Errorf("ValidatePath(%q) accepted", bad)
		}
	}
}
