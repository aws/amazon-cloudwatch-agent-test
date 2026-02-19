// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package profiling

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"sync"
	"time"
)

type SpanCategory string

const (
	CategorySetup      SpanCategory = "setup"
	CategoryAgentWait  SpanCategory = "agent_wait"
	CategoryAPICall    SpanCategory = "api_call"
	CategoryValidation SpanCategory = "validation"
	CategorySleep      SpanCategory = "sleep"
	CategoryCleanup    SpanCategory = "cleanup"
	CategoryOther      SpanCategory = "other"
)

type Span struct {
	Name      string       `json:"name"`
	Category  SpanCategory `json:"category"`
	StartTime time.Time    `json:"start_time"`
	EndTime   time.Time    `json:"end_time"`
	Duration  float64      `json:"duration_seconds"`
}

type TestProfile struct {
	TestName  string    `json:"test_name"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Duration  float64   `json:"duration_seconds"`
	Spans     []Span    `json:"spans"`
}

type Summary struct {
	TotalDuration    float64            `json:"total_duration_seconds"`
	CategoryBreakdown map[SpanCategory]float64 `json:"category_breakdown_seconds"`
	Tests            []TestProfile      `json:"tests"`
}

type Profiler struct {
	mu    sync.Mutex
	tests []TestProfile
}

var global = &Profiler{}

func Global() *Profiler { return global }

type Timer struct {
	profiler *Profiler
	testName string
	span     Span
}

func (p *Profiler) StartTest(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tests = append(p.tests, TestProfile{
		TestName:  name,
		StartTime: time.Now(),
	})
}

func (p *Profiler) EndTest(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := len(p.tests) - 1; i >= 0; i-- {
		if p.tests[i].TestName == name && p.tests[i].EndTime.IsZero() {
			p.tests[i].EndTime = time.Now()
			p.tests[i].Duration = p.tests[i].EndTime.Sub(p.tests[i].StartTime).Seconds()
			return
		}
	}
}

func (p *Profiler) StartSpan(testName, spanName string, category SpanCategory) *Timer {
	return &Timer{
		profiler: p,
		testName: testName,
		span: Span{
			Name:      spanName,
			Category:  category,
			StartTime: time.Now(),
		},
	}
}

func (t *Timer) Stop() {
	if t == nil {
		return
	}
	t.span.EndTime = time.Now()
	t.span.Duration = t.span.EndTime.Sub(t.span.StartTime).Seconds()
	t.profiler.mu.Lock()
	defer t.profiler.mu.Unlock()
	for i := len(t.profiler.tests) - 1; i >= 0; i-- {
		if t.profiler.tests[i].TestName == t.testName {
			t.profiler.tests[i].Spans = append(t.profiler.tests[i].Spans, t.span)
			return
		}
	}
}

func (p *Profiler) Summary() Summary {
	p.mu.Lock()
	defer p.mu.Unlock()

	s := Summary{
		CategoryBreakdown: make(map[SpanCategory]float64),
		Tests:             p.tests,
	}
	for _, t := range p.tests {
		s.TotalDuration += t.Duration
		for _, span := range t.Spans {
			s.CategoryBreakdown[span.Category] += span.Duration
		}
	}
	return s
}

func (p *Profiler) WriteReport(path string) error {
	s := p.Summary()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	p.PrintSummary()
	return nil
}

func (p *Profiler) PrintSummary() {
	s := p.Summary()
	log.Printf("=== TEST PROFILING SUMMARY ===")
	log.Printf("Total duration: %.1fs", s.TotalDuration)
	log.Printf("--- Category Breakdown ---")

	type kv struct {
		k SpanCategory
		v float64
	}
	var sorted []kv
	for k, v := range s.CategoryBreakdown {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].v > sorted[j].v })
	for _, kv := range sorted {
		pct := 0.0
		if s.TotalDuration > 0 {
			pct = (kv.v / s.TotalDuration) * 100
		}
		log.Printf("  %-15s %7.1fs (%5.1f%%)", kv.k, kv.v, pct)
	}

	log.Printf("--- Per-Test Breakdown ---")
	for _, t := range s.Tests {
		log.Printf("  %s: %.1fs", t.TestName, t.Duration)
		for _, span := range t.Spans {
			log.Printf("    %-30s [%-12s] %.1fs", span.Name, span.Category, span.Duration)
		}
	}
	fmt.Println("=== END PROFILING SUMMARY ===")
}
