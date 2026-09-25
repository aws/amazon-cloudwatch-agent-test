// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package metric

import (
	"testing"
	"time"
)

func TestRetryValidation_SuccessOnFirstAttempt(t *testing.T) {
	calls := 0
	ok := RetryValidation(5, 0, func() bool {
		calls++
		return true
	})
	if !ok {
		t.Fatal("expected true, got false")
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestRetryValidation_SuccessAfterKFailures(t *testing.T) {
	tests := []struct {
		name       string
		failCount  int
		maxAttempt int
	}{
		{"success on 2nd attempt", 1, 5},
		{"success on 3rd attempt", 2, 5},
		{"success on last attempt", 4, 5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			ok := RetryValidation(tc.maxAttempt, 0, func() bool {
				calls++
				return calls > tc.failCount
			})
			if !ok {
				t.Fatalf("expected true, got false")
			}
			expectedCalls := tc.failCount + 1
			if calls != expectedCalls {
				t.Fatalf("expected %d calls, got %d", expectedCalls, calls)
			}
		})
	}
}

func TestRetryValidation_AllFail(t *testing.T) {
	const maxAttempts = 5
	calls := 0
	ok := RetryValidation(maxAttempts, 0, func() bool {
		calls++
		return false
	})
	if ok {
		t.Fatal("expected false, got true")
	}
	if calls != maxAttempts {
		t.Fatalf("expected %d calls, got %d", maxAttempts, calls)
	}
}

func TestRetryValidation_DoesNotSleepAfterFinalAttempt(t *testing.T) {
	// With a non-zero interval and maxAttempts=2, if we always fail we should
	// only sleep once (between attempt 1 and 2), not after attempt 2.
	start := time.Now()
	const interval = 10 * time.Millisecond
	RetryValidation(2, interval, func() bool { return false })
	elapsed := time.Since(start)
	// Should have slept ~10ms (1 interval), not ~20ms (2 intervals).
	if elapsed >= 2*interval {
		t.Fatalf("expected ~1 interval sleep, but elapsed %v", elapsed)
	}
}
