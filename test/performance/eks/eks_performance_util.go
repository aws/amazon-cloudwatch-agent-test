// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package eks

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
)

// PerformanceMetrics represents a collection of performance metrics and their associated dimensions
type PerformanceMetrics struct {
	Metrics []Metric `json:"metrics"`
}

// Metric represents a single metric with its name, dimensions map and threshold
type Metric struct {
	Name       string            `json:"name"`
	Dimensions map[string]string `json:"dimensions"`
	Threshold  float64           `json:"threshold"`
	Statistic  string            `json:"stat"`
}

// GetEKSPerformanceMetrics - Gets desired EKS performance metrics based on json file
func GetEKSPerformanceMetrics(performanceMetricMapName string) (*PerformanceMetrics, error) {
	var metricDimensions *PerformanceMetrics

	configPath := filepath.Join("resources/performance_metric_maps", performanceMetricMapName)
	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	err = json.Unmarshal(content, &metricDimensions)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal performance metrics: %v", err)
	}

	return metricDimensions, nil
}

// GetMetricDimensions - Gets dimensions for performance metric
func GetMetricDimensions(metric Metric, env *environment.MetaData) []types.Dimension {
	var dimensions []types.Dimension
	for name, value := range metric.Dimensions {
		if name == "ClusterName" && env.EKSClusterName != "" {
			value = env.EKSClusterName
		}
		dimensions = append(dimensions, types.Dimension{
			Name:  aws.String(name),
			Value: aws.String(value),
		})
	}
	return dimensions
}

// ValidatePerformanceMetrics - Validates that the metric exists and is within the expected threshold (+/- 15%)
func ValidatePerformanceMetrics(name string, threshold float64, stat string, dimensions []types.Dimension) status.TestResult {
	testResult := status.TestResult{
		Name:   name,
		Status: status.FAILED,
	}

	// get metric from cloudwatch container insights namespace
	valueFetcher := metric.MetricValueFetcher{}
	values, err := valueFetcher.Fetch(metric.ContainerInsightsNamespace, name, dimensions, metric.Statistics(stat), metric.MinuteStatPeriod)
	if err != nil {
		log.Println("failed to fetch metrics", err)
		return testResult
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(name, values, threshold) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
