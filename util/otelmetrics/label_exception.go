// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

import (
	"fmt"
	"regexp"
	"regexp/syntax"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// LabelException is a label exception transform: it removes the resource
// attributes under Prefixes, except the keys that Keep matches. The chart renders
// one, transform/cw_k8s_ci_v0_metrics_labels, when an otelFilters include entry
// names a label or a longer prefix inside a built-in prefix, because the
// awsattributelimit processor cannot remove a prefix except some keys. The
// chart then leaves that prefix out of the awsattributelimit denylist.
type LabelException struct {
	Processor string
	Prefixes  []string
	Keep      *regexp.Regexp
}

// Removes reports whether the transform removes the resource attribute key.
func (x LabelException) Removes(key string) bool {
	for _, p := range x.Prefixes {
		if strings.HasPrefix(key, p) {
			return !x.Keep.MatchString(key)
		}
	}
	return false
}

// labelAttributePrefixes are the resource attributes that carry Kubernetes labels.
var labelAttributePrefixes = []string{"k8s.node.label.", "k8s.pod.label."}

func isLabelAttribute(key string) bool {
	for _, p := range labelAttributePrefixes {
		if strings.HasPrefix(key, p) {
			return true
		}
	}
	return false
}

// ottlStringLiteral matches an OTTL string literal, with its quotes.
const ottlStringLiteral = `("(?:[^"\\]|\\.)*")`

// The statements of a label exception transform, in a resource context group
// without conditions:
//
//	set(cache["kept_labels"], attributes)
//	keep_matching_keys(cache["kept_labels"], "^(<keep alternatives>)")
//	delete_matching_keys(attributes, "^<prefix>")   # one or more
//	merge_maps(attributes, cache["kept_labels"], "upsert")
//
// The copy keeps only the keys that the keep regex matches, and the merge puts
// them back after the deletes.
var (
	exceptionCopy   = regexp.MustCompile(`^set\(\s*cache\[` + ottlStringLiteral + `\]\s*,\s*attributes\s*\)$`)
	exceptionKeep   = regexp.MustCompile(`^keep_matching_keys\(\s*cache\[` + ottlStringLiteral + `\]\s*,\s*` + ottlStringLiteral + `\s*\)$`)
	exceptionDelete = regexp.MustCompile(`^delete_matching_keys\(\s*attributes\s*,\s*` + ottlStringLiteral + `\s*\)$`)
	exceptionMerge  = regexp.MustCompile(`^merge_maps\(\s*attributes\s*,\s*cache\[` + ottlStringLiteral + `\]\s*,\s*"upsert"\s*\)$`)

	ottlEditor = regexp.MustCompile(`^([a-z_]+)\(`)
)

// parseLabelExceptions reads the label exception transforms from the transform
// processors. It returns an error that wraps ErrFilterNotModeled for any other
// metric statement that removes or writes k8s.node.label.* or k8s.pod.label.*
// resource attributes. Statements that only read a label, and log statements,
// do not change the labels on metrics.
func parseLabelExceptions(processors map[string]yaml.Node) ([]LabelException, error) {
	var out []LabelException
	for _, name := range sortedProcessors(processors, "transform") {
		node := processors[name]
		var p struct {
			MetricStatements []yaml.Node `yaml:"metric_statements"`
		}
		if err := node.Decode(&p); err != nil {
			return nil, fmt.Errorf("parsing processor %s: %w", name, err)
		}
		for _, entry := range p.MetricStatements {
			var group struct {
				Context    string   `yaml:"context"`
				Statements []string `yaml:"statements"`
				Conditions []string `yaml:"conditions"`
			}
			if entry.Kind == yaml.ScalarNode {
				group.Statements = []string{entry.Value} // a statement outside a context group names its paths in full
			} else if err := entry.Decode(&group); err != nil {
				return nil, fmt.Errorf("parsing processor %s: %w", name, err)
			}
			if group.Context == "resource" && len(group.Conditions) == 0 {
				if x, ok := matchLabelException(group.Statements); ok {
					x.Processor = name
					out = append(out, x)
					continue
				}
			}
			for _, s := range group.Statements {
				if reason := labelStatementEffect(group.Context, s); reason != "" {
					return nil, fmt.Errorf("%w: processor %s: the statement %s %s, and the statement group is not a label exception transform",
						ErrFilterNotModeled, name, strings.TrimSpace(s), reason)
				}
			}
		}
	}
	return out, nil
}

// matchLabelException returns the transform that statements form, if they have
// the label exception shape.
func matchLabelException(statements []string) (LabelException, bool) {
	n := len(statements)
	if n < 4 {
		return LabelException{}, false
	}
	copied := exceptionCopy.FindStringSubmatch(strings.TrimSpace(statements[0]))
	keep := exceptionKeep.FindStringSubmatch(strings.TrimSpace(statements[1]))
	merged := exceptionMerge.FindStringSubmatch(strings.TrimSpace(statements[n-1]))
	if copied == nil || keep == nil || merged == nil || keep[1] != copied[1] || merged[1] != copied[1] {
		return LabelException{}, false
	}
	keepPattern, err := strconv.Unquote(keep[2])
	if err != nil {
		return LabelException{}, false
	}
	keepRegex, err := regexp.Compile(keepPattern)
	if err != nil {
		return LabelException{}, false
	}
	x := LabelException{Keep: keepRegex}
	for _, s := range statements[2 : n-1] {
		m := exceptionDelete.FindStringSubmatch(strings.TrimSpace(s))
		if m == nil {
			return LabelException{}, false
		}
		pattern, err := strconv.Unquote(m[1])
		if err != nil {
			return LabelException{}, false
		}
		prefix, ok := literalPrefixPattern(pattern)
		if !ok {
			return LabelException{}, false
		}
		x.Prefixes = append(x.Prefixes, prefix)
	}
	return x, true
}

// literalPrefixPattern returns p when pattern is "^" followed by the literal p.
func literalPrefixPattern(pattern string) (string, bool) {
	re, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return "", false
	}
	re = re.Simplify()
	if re.Op != syntax.OpConcat || len(re.Sub) != 2 || re.Sub[0].Op != syntax.OpBeginText ||
		re.Sub[1].Op != syntax.OpLiteral || re.Sub[1].Flags&syntax.FoldCase != 0 {
		return "", false
	}
	return string(re.Sub[1].Rune), true
}

// labelStatementEffect describes how an OTTL statement in context removes or
// writes label resource attributes, or returns "" when it does not. An editor
// changes its first argument, so only that argument is read.
func labelStatementEffect(context, statement string) string {
	s := strings.TrimSpace(statement)
	m := ottlEditor.FindStringSubmatch(s)
	if m == nil {
		if strings.Contains(s, ".label.") {
			return "is not an editor call this model can read"
		}
		return ""
	}
	editor := m[1]
	args := splitOTTLArgs(s[len(m[0]):])
	if len(args) == 0 {
		return ""
	}
	target := args[0]
	switch {
	case target == "resource.attributes" || (context == "resource" && target == "attributes"):
		// The editor changes the resource attributes as a whole.
	case strings.HasPrefix(target, "resource.attributes["), context == "resource" && strings.HasPrefix(target, "attributes["):
		key, err := strconv.Unquote(strings.TrimSuffix(target[strings.Index(target, "[")+1:], "]"))
		if err != nil || isLabelAttribute(key) {
			return "writes a label resource attribute"
		}
		return ""
	default:
		return "" // not the resource attributes
	}
	switch editor {
	case "delete_key":
		if len(args) < 2 {
			return "has no key"
		}
		key, err := strconv.Unquote(args[1])
		if err != nil || isLabelAttribute(key) {
			return "deletes a label resource attribute"
		}
		return ""
	case "delete_matching_keys":
		if len(args) < 2 {
			return "has no pattern"
		}
		pattern, err := strconv.Unquote(args[1])
		if err != nil {
			return "has a pattern this model cannot read"
		}
		prefix, ok := literalPrefixPattern(pattern)
		if !ok {
			return "deletes the resource attributes that a pattern this model cannot read matches"
		}
		for _, p := range labelAttributePrefixes {
			if strings.HasPrefix(prefix, p) || strings.HasPrefix(p, prefix) {
				return "deletes label resource attributes"
			}
		}
		return ""
	default:
		return fmt.Sprintf("changes the resource attributes with %s", editor)
	}
}

// splitOTTLArgs splits the arguments of a call, given the text after its "(",
// at the top-level commas, up to the closing parenthesis.
func splitOTTLArgs(s string) []string {
	var args []string
	depth, start, quoted := 0, 0, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quoted && c == '\\':
			i++
		case c == '"':
			quoted = !quoted
		case quoted:
		case c == '(' || c == '[' || c == '{':
			depth++
		case c == ')' || c == ']' || c == '}':
			if depth == 0 {
				return append(args, strings.TrimSpace(s[start:i]))
			}
			depth--
		case c == ',' && depth == 0:
			args = append(args, strings.TrimSpace(s[start:i]))
			start = i + 1
		}
	}
	return append(args, strings.TrimSpace(s[start:]))
}
