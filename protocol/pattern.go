package protocol

import (
	"fmt"
	"regexp"
	"strings"
)

var paramRe = regexp.MustCompile(`\{(\w+)\}`)

// Pattern is a topic pattern such as "room:{room_name}". A parameter stops at
// ':', so "run:{id}" cannot swallow a following segment.
type Pattern struct {
	raw    string
	params []string
	re     *regexp.Regexp
}

// ParsePattern compiles a topic pattern.
func ParsePattern(pattern string) (*Pattern, error) {
	if pattern == "" {
		return nil, fmt.Errorf("protocol: empty topic pattern")
	}
	p := &Pattern{raw: pattern}
	var expr strings.Builder
	expr.WriteString("^")
	last := 0
	seen := map[string]bool{}
	for _, loc := range paramRe.FindAllStringSubmatchIndex(pattern, -1) {
		expr.WriteString(regexp.QuoteMeta(pattern[last:loc[0]]))
		name := pattern[loc[2]:loc[3]]
		if seen[name] {
			return nil, fmt.Errorf("protocol: topic pattern %q repeats parameter %q", pattern, name)
		}
		seen[name] = true
		expr.WriteString("([^:]+)")
		p.params = append(p.params, name)
		last = loc[1]
	}
	rest := pattern[last:]
	if strings.ContainsAny(rest, "{}") || strings.ContainsAny(paramRe.ReplaceAllString(pattern, ""), "{}") {
		return nil, fmt.Errorf("protocol: malformed topic pattern %q", pattern)
	}
	expr.WriteString(regexp.QuoteMeta(rest) + "$")
	p.re = regexp.MustCompile(expr.String())
	return p, nil
}

// MustParsePattern is ParsePattern that panics.
func MustParsePattern(pattern string) *Pattern {
	p, err := ParsePattern(pattern)
	if err != nil {
		panic(err)
	}
	return p
}

func (p *Pattern) String() string { return p.raw }

// Params returns the parameter names in order.
func (p *Pattern) Params() []string { return append([]string(nil), p.params...) }

// Match returns a topic's parameters, or false when it does not match.
func (p *Pattern) Match(topic string) (map[string]string, bool) {
	m := p.re.FindStringSubmatch(topic)
	if m == nil {
		return nil, false
	}
	out := make(map[string]string, len(p.params))
	for i, name := range p.params {
		out[name] = m[i+1]
	}
	return out, true
}

// Format fills the pattern, failing on a missing parameter.
func (p *Pattern) Format(params map[string]string) (string, error) {
	var missing []string
	out := paramRe.ReplaceAllStringFunc(p.raw, func(s string) string {
		name := s[1 : len(s)-1]
		v, ok := params[name]
		if !ok {
			missing = append(missing, name)
		}
		return v
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("protocol: topic pattern %q needs parameters %v", p.raw, missing)
	}
	return out, nil
}

// ValidatePath checks a route path: "/ws/chat/{room}", each parameter a whole
// segment. Wildcards ("{rest...}") are refused: each router names and slices
// them differently, so the same route would see different values.
func ValidatePath(path string) error {
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("protocol: route path %q must start with /", path)
	}
	seen := map[string]bool{}
	for _, seg := range strings.Split(path[1:], "/") {
		if !strings.ContainsAny(seg, "{}") {
			continue
		}
		m := pathSegmentRe.FindStringSubmatch(seg)
		if m == nil {
			return fmt.Errorf("protocol: route path %q: segment %q must be a literal or a single {name}", path, seg)
		}
		if seen[m[1]] {
			return fmt.Errorf("protocol: route path %q repeats parameter %q", path, m[1])
		}
		seen[m[1]] = true
	}
	return nil
}

var pathSegmentRe = regexp.MustCompile(`^\{(\w+)\}$`)

// PathParams returns the {name} parameters of a route path ("/ws/chat/{room}"),
// the form net/http and AsyncAPI share.
func PathParams(path string) []string {
	var out []string
	for _, m := range pathParamRe.FindAllStringSubmatch(path, -1) {
		out = append(out, m[1])
	}
	return out
}

var pathParamRe = regexp.MustCompile(`\{(\w+)\}`)

// PathRegexp compiles a route path into a matcher with one named group per
// parameter, each matching one segment. When suffix is true the path may be
// preceded by anything, for routers that mount it under a prefix.
func PathRegexp(path string, suffix bool) *regexp.Regexp {
	var expr strings.Builder
	if suffix {
		expr.WriteString("(?:^|.*?)")
	} else {
		expr.WriteString("^")
	}
	last := 0
	for _, loc := range pathParamRe.FindAllStringSubmatchIndex(path, -1) {
		expr.WriteString(regexp.QuoteMeta(path[last:loc[0]]))
		expr.WriteString("(?P<" + path[loc[2]:loc[3]] + ">[^/]+)")
		last = loc[1]
	}
	expr.WriteString(regexp.QuoteMeta(path[last:]) + "/?$")
	return regexp.MustCompile(expr.String())
}

// ColonPath converts "/ws/chat/{room}" to "/ws/chat/:room" for routers that use
// that syntax (gin, echo, Fiber).
func ColonPath(path string) string { return pathParamRe.ReplaceAllString(path, ":$1") }
