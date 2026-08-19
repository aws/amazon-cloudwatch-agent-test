// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package metric

import "time"

// RevalidateMaxAttempts is the number of times to query CloudWatch before
// declaring final failure. Chosen to allow up to ~4 minutes of additional
// propagation time beyond the initial sleep, which is sufficient for the
// majority of late-arriving (metric, dimension-set) pairs observed in CI.
const RevalidateMaxAttempts = 5

// RevalidateInterval is the pause between consecutive re-validation attempts.
// 60 seconds balances giving CloudWatch time to surface late dimension sets
// against keeping the overall test duration reasonable.
const RevalidateInterval = 60 * time.Second

// RetryValidation performs bounded, spaced re-validation of a metric query.
//
// It calls validate up to maxAttempts times, sleeping interval between
// consecutive attempts (but NOT after the final attempt). It returns true as
// soon as validate returns true, or false after all attempts are exhausted.
//
// This addresses the root cause of flaky EKS metric_value_benchmark failures:
// CloudWatch propagates some (metric, dimension-set) pairs after the initial
// query window, causing all-or-nothing validation to fail even though the
// agent is correctly publishing. Re-querying with spacing catches late
// arrivals without masking genuine regressions.
func RetryValidation(maxAttempts int, interval time.Duration, validate func() bool) bool {
	for i := 0; i < maxAttempts; i++ {
		if validate() {
			return true
		}
		// Do not sleep after the final attempt.
		if i < maxAttempts-1 {
			time.Sleep(interval)
		}
	}
	return false
}
