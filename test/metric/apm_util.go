// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric

import (
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/aws-sdk-go-v2/aws"
)

var (
	APMMetricNames = []string{
		"Error",
		"Fault",
		"Latency",
	}

	ServerConsumerInstructions = []dimension.Instruction{
		{
			Key:   "EKS.Cluster",
			Value: dimension.UnknownDimensionValue(),
		},
		{
			Key:   "K8s.Namespace",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("default")},
		},
		{
			Key:   "Service",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("service-name")},
		},
		{
			Key:   "Operation",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("operation")},
		},
	}

	ClientProducerInstructions = []dimension.Instruction{
		{
			Key:   "EKS.Cluster",
			Value: dimension.UnknownDimensionValue(),
		},
		{
			Key:   "K8s.Namespace",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("default")},
		},
		{
			Key:   "Service",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("service-name")},
		},
		{
			Key:   "RemoteService",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("service-name-remote")},
		},
		{
			Key:   "Operation",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("operation")},
		},
		{
			Key:   "RemoteOperation",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("remote-operation")},
		},
		{
			Key:   "RemoteTarget",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("remote-target")},
		},
	}
)

func ValidateAPMMetric(dimFactory dimension.Factory, namespace string, metricName string, instructions []dimension.Instruction) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	dims, failed := dimFactory.GetDimensions(instructions)
	if len(failed) > 0 {
		return testResult
	}

	fetcher := MetricValueFetcher{}
	values, err := fetcher.Fetch(namespace, metricName, dims, SUM, HighResolutionStatPeriod)
	if err != nil {
		return testResult
	}

	if !IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 0) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
