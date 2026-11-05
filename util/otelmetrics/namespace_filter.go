// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const namespaceAttribute = "k8s.namespace.name"

// NamespaceFilter is the metric conditions of the filter processors in a
// collector configuration that read k8s.namespace.name. A series whose
// namespace makes any condition true is dropped.
type NamespaceFilter struct {
	Processors []string
	conditions []ottlNode
}

// ParseNamespaceFilter reads every metric and datapoint condition that names
// k8s.namespace.name from the filter processors of a rendered collector
// configuration. A condition outside the subset of OTTL that the chart renders
// (and, or, not, ==, != and IsMatch over resource.attributes["k8s.namespace.name"],
// string literals, true, false and nil) returns an error that wraps
// ErrFilterNotModeled. A configuration without such conditions has an empty
// filter that drops nothing.
func ParseNamespaceFilter(otelConfigYAML string) (NamespaceFilter, error) {
	var cfg struct {
		Processors map[string]yaml.Node `yaml:"processors"`
	}
	if err := yaml.Unmarshal([]byte(otelConfigYAML), &cfg); err != nil {
		return NamespaceFilter{}, fmt.Errorf("parsing collector configuration: %w", err)
	}
	var f NamespaceFilter
	for _, name := range sortedProcessors(cfg.Processors, "filter") {
		node := cfg.Processors[name]
		var p struct {
			Metrics struct {
				Metric    []string `yaml:"metric"`
				Datapoint []string `yaml:"datapoint"`
			} `yaml:"metrics"`
		}
		if err := node.Decode(&p); err != nil {
			return NamespaceFilter{}, fmt.Errorf("parsing processor %s: %w", name, err)
		}
		found := false
		for _, condition := range append(p.Metrics.Metric, p.Metrics.Datapoint...) {
			if !strings.Contains(condition, namespaceAttribute) {
				continue
			}
			n, err := parseOTTLCondition(condition)
			if err != nil {
				return NamespaceFilter{}, fmt.Errorf("%w: processor %s condition %q: %v", ErrFilterNotModeled, name, condition, err)
			}
			f.conditions = append(f.conditions, n)
			found = true
		}
		if found {
			f.Processors = append(f.Processors, name)
		}
	}
	return f, nil
}

// Drops reports whether the filter drops the series of a namespace.
func (f NamespaceFilter) Drops(namespace string) bool {
	for _, c := range f.conditions {
		if c.eval(namespace).b {
			return true
		}
	}
	return false
}

// Active reports whether the filter has any condition.
func (f NamespaceFilter) Active() bool {
	return len(f.conditions) > 0
}

// ---------------------------------------------------------------------------
// The OTTL subset. Each node evaluates for a series whose namespace is set, so
// the namespace path is always a string and never nil.
// ---------------------------------------------------------------------------

type ottlKind int

const (
	ottlBool ottlKind = iota
	ottlString
	ottlNil
)

type ottlValue struct {
	kind ottlKind
	b    bool
	s    string
}

type ottlNode struct {
	kind ottlKind
	eval func(namespace string) ottlValue
}

type ottlToken struct {
	text  string // an operator, a parenthesis, a comma or a name
	str   string // the value of a string literal
	isStr bool
	index string // the key of a name["key"]
}

func tokenizeOTTL(s string) ([]ottlToken, error) {
	var tokens []ottlToken
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n':
			i++
		case c == '(' || c == ')' || c == ',':
			tokens = append(tokens, ottlToken{text: string(c)})
			i++
		case strings.HasPrefix(s[i:], "==") || strings.HasPrefix(s[i:], "!="):
			tokens = append(tokens, ottlToken{text: s[i : i+2]})
			i += 2
		case c == '"':
			lit, n, err := scanOTTLString(s[i:])
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, ottlToken{str: lit, isStr: true})
			i += n
		case c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z':
			j := i
			for j < len(s) && (s[j] == '_' || s[j] == '.' || s[j] >= '0' && s[j] <= '9' || s[j] >= 'A' && s[j] <= 'Z' || s[j] >= 'a' && s[j] <= 'z') {
				j++
			}
			tok := ottlToken{text: s[i:j]}
			if j < len(s) && s[j] == '[' {
				lit, n, err := scanOTTLString(s[j+1:])
				if err != nil || j+1+n >= len(s) || s[j+1+n] != ']' {
					return nil, fmt.Errorf("unsupported index after %s", tok.text)
				}
				tok.index = lit
				j += n + 2
			}
			tokens = append(tokens, tok)
			i = j
		default:
			return nil, fmt.Errorf("unsupported character %q", c)
		}
	}
	return tokens, nil
}

// scanOTTLString reads the string literal at the start of s and returns its
// value and its length in s.
func scanOTTLString(s string) (string, int, error) {
	if !strings.HasPrefix(s, `"`) {
		return "", 0, fmt.Errorf("want a string literal")
	}
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			v, err := strconv.Unquote(s[:i+1])
			return v, i + 1, err
		}
	}
	return "", 0, fmt.Errorf("unterminated string literal")
}

type ottlParser struct {
	tokens []ottlToken
	pos    int
}

func parseOTTLCondition(condition string) (ottlNode, error) {
	tokens, err := tokenizeOTTL(condition)
	if err != nil {
		return ottlNode{}, err
	}
	p := &ottlParser{tokens: tokens}
	n, err := p.or()
	if err != nil {
		return ottlNode{}, err
	}
	if p.pos != len(p.tokens) {
		return ottlNode{}, fmt.Errorf("unexpected %q", p.tokens[p.pos].text)
	}
	if n.kind != ottlBool {
		return ottlNode{}, fmt.Errorf("the condition is not a boolean")
	}
	return n, nil
}

func (p *ottlParser) peek(text string) bool {
	return p.pos < len(p.tokens) && !p.tokens[p.pos].isStr && p.tokens[p.pos].index == "" && p.tokens[p.pos].text == text
}

func (p *ottlParser) expect(text string) error {
	if !p.peek(text) {
		return fmt.Errorf("want %q", text)
	}
	p.pos++
	return nil
}

func (p *ottlParser) or() (ottlNode, error) {
	return p.binary("or", p.and, func(a, b bool) bool { return a || b })
}

func (p *ottlParser) and() (ottlNode, error) {
	return p.binary("and", p.not, func(a, b bool) bool { return a && b })
}

func (p *ottlParser) binary(op string, next func() (ottlNode, error), combine func(a, b bool) bool) (ottlNode, error) {
	left, err := next()
	if err != nil {
		return ottlNode{}, err
	}
	for p.peek(op) {
		p.pos++
		right, err := next()
		if err != nil {
			return ottlNode{}, err
		}
		if left.kind != ottlBool || right.kind != ottlBool {
			return ottlNode{}, fmt.Errorf("%q needs boolean operands", op)
		}
		l, r := left.eval, right.eval
		left = ottlNode{kind: ottlBool, eval: func(ns string) ottlValue {
			return ottlValue{kind: ottlBool, b: combine(l(ns).b, r(ns).b)}
		}}
	}
	return left, nil
}

func (p *ottlParser) not() (ottlNode, error) {
	if !p.peek("not") {
		return p.comparison()
	}
	p.pos++
	n, err := p.not()
	if err != nil {
		return ottlNode{}, err
	}
	if n.kind != ottlBool {
		return ottlNode{}, fmt.Errorf(`"not" needs a boolean operand`)
	}
	return ottlNode{kind: ottlBool, eval: func(ns string) ottlValue {
		return ottlValue{kind: ottlBool, b: !n.eval(ns).b}
	}}, nil
}

func (p *ottlParser) comparison() (ottlNode, error) {
	left, err := p.operand()
	if err != nil {
		return ottlNode{}, err
	}
	if !p.peek("==") && !p.peek("!=") {
		return left, nil
	}
	equal := p.tokens[p.pos].text == "=="
	p.pos++
	right, err := p.operand()
	if err != nil {
		return ottlNode{}, err
	}
	return ottlNode{kind: ottlBool, eval: func(ns string) ottlValue {
		return ottlValue{kind: ottlBool, b: (left.eval(ns) == right.eval(ns)) == equal}
	}}, nil
}

func (p *ottlParser) operand() (ottlNode, error) {
	if p.pos >= len(p.tokens) {
		return ottlNode{}, fmt.Errorf("unexpected end of condition")
	}
	tok := p.tokens[p.pos]
	p.pos++
	constant := func(v ottlValue) ottlNode {
		return ottlNode{kind: v.kind, eval: func(string) ottlValue { return v }}
	}
	switch {
	case tok.isStr:
		return constant(ottlValue{kind: ottlString, s: tok.str}), nil
	case tok.index != "":
		if tok.text != "resource.attributes" || tok.index != namespaceAttribute {
			return ottlNode{}, fmt.Errorf("unsupported path %s[%q]", tok.text, tok.index)
		}
		return ottlNode{kind: ottlString, eval: func(ns string) ottlValue { return ottlValue{kind: ottlString, s: ns} }}, nil
	case tok.text == "(":
		n, err := p.or()
		if err != nil {
			return ottlNode{}, err
		}
		return n, p.expect(")")
	case tok.text == "true" || tok.text == "false":
		return constant(ottlValue{kind: ottlBool, b: tok.text == "true"}), nil
	case tok.text == "nil":
		return constant(ottlValue{kind: ottlNil}), nil
	case tok.text == "IsMatch":
		return p.isMatch()
	default:
		return ottlNode{}, fmt.Errorf("unsupported %q", tok.text)
	}
}

// isMatch parses the arguments of IsMatch(target, "pattern"). Like OTTL, it
// matches anywhere in the target unless the pattern is anchored.
func (p *ottlParser) isMatch() (ottlNode, error) {
	if err := p.expect("("); err != nil {
		return ottlNode{}, err
	}
	target, err := p.operand()
	if err != nil {
		return ottlNode{}, err
	}
	if err := p.expect(","); err != nil {
		return ottlNode{}, err
	}
	if p.pos >= len(p.tokens) || !p.tokens[p.pos].isStr {
		return ottlNode{}, fmt.Errorf("IsMatch needs a string literal pattern")
	}
	re, err := regexp.Compile(p.tokens[p.pos].str)
	if err != nil {
		return ottlNode{}, fmt.Errorf("IsMatch pattern: %w", err)
	}
	p.pos++
	if err := p.expect(")"); err != nil {
		return ottlNode{}, err
	}
	if target.kind != ottlString {
		return ottlNode{}, fmt.Errorf("IsMatch needs a string target")
	}
	return ottlNode{kind: ottlBool, eval: func(ns string) ottlValue {
		return ottlValue{kind: ottlBool, b: re.MatchString(target.eval(ns).s)}
	}}, nil
}
