// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package models

import "time"

// ValidatorFactory will be an interface for every validator and signals the validation process.
// https://github.com/aws/amazon-cloudwatch-agent-test/blob/c5b8bd2da8e71f7ae4db0b66dccffe07dc429fae/validator/validators/validator.go#L43-L60

type ValidatorFactory interface {
	// GenerateLoad will send the metrics/logs/traces load to CloudWatchAgent (e.g sending 1000 statsd metrics to CWA to monitor)
	GenerateLoad() error
	// CheckData will get metrics defined by the generator yaml and validate the required metrics
	// (e.g https://github.com/aws/amazon-cloudwatch-agent-test/blob/c5b8bd2da8e71f7ae4db0b66dccffe07dc429fae/test/stress/statsd/parameters.yml#L21-L66)
	CheckData(startTime, endTime time.Time) error

	// Cleanup will clean up all the resources created by the validator.
	Cleanup() error
}
