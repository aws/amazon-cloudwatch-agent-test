// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package otelmetrics

import (
	"errors"
	"fmt"
	"regexp"
	"regexp/syntax"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ErrFilterNotModeled marks a filter in a collector configuration that the
// reconciliation cannot model, so a test that depends on it cannot tell which
// labels or objects the filter removes.
var ErrFilterNotModeled = errors.New("filter not modeled")

// LabelExtraction is the label extraction of the k8sattributes processor for one
// source, "node" or "pod": the labels it copies onto a series, each as Prefix
// plus the label key. Build it with ParseLabelExtraction; the zero value
// extracts nothing.
type LabelExtraction struct {
	From      string
	Prefix    string // "k8s.<From>.label."
	Processor string
	keys      map[string]struct{}
	regexes   []*regexp.Regexp
	all       bool
}

// labelRuleFields are the fields of an extraction rule the model covers. Any
// other field, such as a value regex, changes the attribute or its value.
var labelRuleFields = map[string]bool{"tag_name": true, "key": true, "key_regex": true, "from": true}

// keyRegexTagSuffix is a tag name back reference to the first group, with the
// configuration's $ escapes.
var keyRegexTagSuffix = regexp.MustCompile(`^\$+(1|\{1\})$`)

// ParseLabelExtraction reads the k8sattributes label rules for from ("node" or
// "pod") from a rendered collector configuration. A rule must write the label
// key unchanged under the prefix k8s.<from>.label., as the chart renders it: a
// key with that tag name, or a key_regex whose single group spans the key, with
// a back reference to it. Any other rule, or rules for from in more than one
// processor, returns an error that wraps ErrFilterNotModeled. A configuration
// without rules for from is an error.
func ParseLabelExtraction(otelConfigYAML, from string) (LabelExtraction, error) {
	var cfg struct {
		Processors map[string]yaml.Node `yaml:"processors"`
	}
	if err := yaml.Unmarshal([]byte(otelConfigYAML), &cfg); err != nil {
		return LabelExtraction{}, fmt.Errorf("parsing collector configuration: %w", err)
	}
	e := LabelExtraction{From: from, Prefix: "k8s." + from + ".label.", keys: map[string]struct{}{}}
	var processors []string
	for _, name := range sortedProcessors(cfg.Processors, "k8sattributes") {
		node := cfg.Processors[name]
		var p struct {
			Extract struct {
				Labels []map[string]string `yaml:"labels"`
			} `yaml:"extract"`
		}
		if err := node.Decode(&p); err != nil {
			return LabelExtraction{}, fmt.Errorf("parsing processor %s: %w", name, err)
		}
		found := false
		for _, rule := range p.Extract.Labels {
			if src := rule["from"]; src != from && !(src == "" && from == "pod") {
				continue // k8sattributes reads a rule without from from the pod
			}
			found = true
			if err := e.addRule(rule); err != nil {
				return LabelExtraction{}, fmt.Errorf("processor %s: %w", name, err)
			}
		}
		if found {
			processors = append(processors, name)
		}
	}
	switch len(processors) {
	case 0:
		return LabelExtraction{}, fmt.Errorf("no k8sattributes processor extracts %s labels", from)
	case 1:
		e.Processor = processors[0]
		return e, nil
	default:
		return LabelExtraction{}, fmt.Errorf("%w: %s labels are extracted by more than one k8sattributes processor %v, and which pipeline runs which is not modeled",
			ErrFilterNotModeled, from, processors)
	}
}

func (e *LabelExtraction) addRule(rule map[string]string) error {
	var unknown []string
	for field := range rule {
		if !labelRuleFields[field] {
			unknown = append(unknown, field)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return fmt.Errorf("%w: label rule %v sets %v", ErrFilterNotModeled, rule, unknown)
	}
	key, keyRegex, tag := rule["key"], rule["key_regex"], rule["tag_name"]
	switch {
	case key != "" && keyRegex == "":
		if tag != e.Prefix+key {
			return fmt.Errorf("%w: label rule for key %q has tag_name %q, not %q", ErrFilterNotModeled, key, tag, e.Prefix+key)
		}
		e.keys[key] = struct{}{}
	case keyRegex != "" && key == "":
		re, err := regexp.Compile("^(?:" + keyRegex + ")$") // as k8sattributes compiles it
		if err != nil {
			return fmt.Errorf("label rule key_regex %q: %w", keyRegex, err)
		}
		tree, err := syntax.Parse(keyRegex, syntax.Perl)
		if err != nil {
			return fmt.Errorf("label rule key_regex %q: %w", keyRegex, err)
		}
		if tree.Op != syntax.OpCapture || tree.Cap != 1 {
			return fmt.Errorf("%w: label rule key_regex %q is not one group around the whole key", ErrFilterNotModeled, keyRegex)
		}
		if !strings.HasPrefix(tag, e.Prefix) || !keyRegexTagSuffix.MatchString(strings.TrimPrefix(tag, e.Prefix)) {
			return fmt.Errorf("%w: label rule key_regex %q has tag_name %q, not %q followed by a reference to the group",
				ErrFilterNotModeled, keyRegex, tag, e.Prefix)
		}
		e.regexes = append(e.regexes, re)
		if sub := tree.Sub[0]; sub.Op == syntax.OpStar && (sub.Sub[0].Op == syntax.OpAnyChar || sub.Sub[0].Op == syntax.OpAnyCharNotNL) {
			e.all = true
		}
	default:
		return fmt.Errorf("label rule %v must set exactly one of key and key_regex", rule)
	}
	return nil
}

// Extracts reports whether a rule copies the label key onto a series.
func (e LabelExtraction) Extracts(key string) bool {
	if _, ok := e.keys[key]; ok {
		return true
	}
	for _, re := range e.regexes {
		if re.MatchString(key) {
			return true
		}
	}
	return false
}

// ExtractsAll reports whether a rule copies every label, as the chart does when
// no include list narrows the extraction.
func (e LabelExtraction) ExtractsAll() bool {
	return e.all
}

// sortedProcessors returns the names of the processors of type typ, sorted.
func sortedProcessors(processors map[string]yaml.Node, typ string) []string {
	var names []string
	for name := range processors {
		if name == typ || strings.HasPrefix(name, typ+"/") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}
