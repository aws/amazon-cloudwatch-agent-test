// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package profiling

import (
	"testing"
	"time"
)

// T wraps a test with profiling spans. No-op when profiling is disabled.
type T struct {
	t       *testing.T
	name    string
	enabled bool
}

// Wrap returns a profiling-aware wrapper around testing.T.
func Wrap(t *testing.T) *T {
	name := t.Name()
	enabled := Enabled()
	if enabled {
		Global().StartTest(name)
		t.Cleanup(func() { Global().EndTest(name) })
	}
	return &T{t: t, name: name, enabled: enabled}
}

// Span records a named span. Call Stop() on the returned Timer when done.
func (pt *T) Span(spanName string, category SpanCategory) *Timer {
	if !pt.enabled {
		return nil
	}
	return Global().StartSpan(pt.name, spanName, category)
}

// Sleep is a profiled replacement for time.Sleep.
func (pt *T) Sleep(d time.Duration, reason string) {
	if pt.enabled {
		timer := pt.Span(reason, CategorySleep)
		defer timer.Stop()
	}
	time.Sleep(d)
}
