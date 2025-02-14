// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric_value_benchmark/eks_resources"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

const (
	dimDelimiter               = "-"
	ContainerInsightsNamespace = "ContainerInsights"
)

type dimToMetrics struct {
	// dim keys as string with dimDelimiter(-) eg. ClusterName-Namespace
	dimStr string
	// metric names to their dimensions with values. Dimension sets will be used for metric data validations
	metrics map[string][][]types.Dimension
}

func ValidateMetrics(env *environment.MetaData, metricFilter string, expectedDimsToMetrics map[string][]string) []status.TestResult {
	var results []status.TestResult
	dimsToMetrics := getMetricsInClusterDimension(env, metricFilter)
	for dims, metrics := range expectedDimsToMetrics {
		var actual map[string][][]types.Dimension
		// find matching dim set from fetched and processed metric-dims groups
		for _, dtm := range dimsToMetrics {
			if dtm.dimStr == dims {
				actual = dtm.metrics
				break
			}
		}
		// expected dim set doesn't exist
		if len(actual) < 1 {
			results = append(results, status.TestResult{
				Name:   dims,
				Status: status.FAILED,
			})
			log.Printf("ValidateMetrics failed with missing dimension set: %s", dims)
			// keep testing other dims or fail early?
			continue
		}
		results = append(results, validateMetricsAvailability(dims, metrics, actual))
		for _, m := range metrics {
			// this is to prevent panic with rand.Intn when metrics are not yet ready in a cluster
			if _, ok := actual[m]; !ok {
				results = append(results, status.TestResult{
					Name:   dims,
					Status: status.FAILED,
				})
				log.Printf("ValidateMetrics failed with missing metric: %s", m)
				continue
			}
			// pick a random dimension set to test metric data OR test all dimension sets which might be overkill
			randIdx := rand.Intn(len(actual[m]))
			results = append(results, validateMetricValue(m, actual[m][randIdx]))
		}
	}
	return results
}

func getMetricsInClusterDimension(env *environment.MetaData, metricFilter string) []dimToMetrics { //map[string]map[string]interface{} {
	listFetcher := Fetcher{}
	log.Printf("Fetching by cluster dimension")
	dims := []types.Dimension{
		{
			Name:  aws.String("ClusterName"),
			Value: aws.String(env.EKSClusterName),
		},
	}
	metrics, err := listFetcher.Fetch(ContainerInsightsNamespace, "", dims)
	if err != nil {
		log.Println("failed to fetch metric list", err)
		return nil
	}
	if len(metrics) < 1 {
		log.Println("cloudwatch metric list is empty")
		return nil
	}

	var results []dimToMetrics
	for _, m := range metrics {
		// filter by metric name filter
		if metricFilter != "" && !strings.Contains(*m.MetricName, metricFilter) {
			continue
		}
		var dims []string
		for _, d := range m.Dimensions {
			dims = append(dims, *d.Name)
		}
		sort.Sort(sort.StringSlice(dims))
		dimsKey := strings.Join(dims, dimDelimiter)
		log.Printf("processing dims: %s", dimsKey)

		var dtm dimToMetrics
		for _, ele := range results {
			if ele.dimStr == dimsKey {
				dtm = ele
				break
			}
		}
		if dtm.dimStr == "" {
			dtm = dimToMetrics{
				dimStr:  dimsKey,
				metrics: make(map[string][][]types.Dimension),
			}
			results = append(results, dtm)
		}
		dtm.metrics[*m.MetricName] = append(dtm.metrics[*m.MetricName], m.Dimensions)
	}
	return results
}

func validateMetricsAvailability(dims string, expected []string, actual map[string][][]types.Dimension) status.TestResult {
	testResult := status.TestResult{
		Name:   dims,
		Status: status.FAILED,
	}
	if compareMetrics(expected, actual) {
		testResult.Status = status.SUCCESSFUL
	} else {
		log.Printf("validateMetricsAvailability failed for %s", dims)
	}
	return testResult
}

func compareMetrics(expected []string, actual map[string][][]types.Dimension) bool {
	if len(expected) != len(actual) {
		log.Printf("the count of fetched metrics do not match with expected count: expected-%v, actual-%v\n", len(expected), len(actual))

		expectedSet := make(map[string]struct{})
		for _, key := range expected {
			expectedSet[key] = struct{}{}
		}

		for key := range actual {
			if _, exists := expectedSet[key]; !exists {
				log.Printf("Unexpected metric in actual output : %s\n", key)
			}
		}

		// Find missing metrics in expected output
		for _, key := range expected {
			if _, exists := actual[key]; !exists {
				log.Printf("Missing metric in actual output: %s\n", key)
			}
		}
		return false
	}

	for _, key := range expected {
		if _, ok := actual[key]; !ok {
			log.Printf("Missing metric in actual: %s\n", key)
			return false
		}
	}
	return true
}

func validateMetricValue(name string, dims []types.Dimension) status.TestResult {
	log.Printf("validateMetricValue with metric: %s", name)
	testResult := status.TestResult{
		Name:   name,
		Status: status.FAILED,
	}
	valueFetcher := MetricValueFetcher{}
	values, err := valueFetcher.Fetch(ContainerInsightsNamespace, name, dims, SAMPLE_COUNT, MinuteStatPeriod)
	if err != nil {
		log.Println("failed to fetch metrics", err)
		return testResult
	}

	if !IsAllValuesGreaterThanOrEqualToExpectedValue(name, values, 0) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func ValidateLogs(env *environment.MetaData) status.TestResult {
	testResult := status.TestResult{
		Name:   "emf-logs",
		Status: status.FAILED,
	}

	end := time.Now()
	start := end.Add(time.Duration(-3) * time.Minute)
	group := fmt.Sprintf("/aws/containerinsights/%s/performance", env.EKSClusterName)

	// need to get the instances used for the EKS cluster
	eKSInstances, err := awsservice.GetEKSInstances(env.EKSClusterName)
	if err != nil {
		log.Println("failed to get EKS instances", err)
		return testResult
	}

	for _, instance := range eKSInstances {
		stream := *instance.InstanceName
		err = awsservice.ValidateLogs(
			group,
			stream,
			&start,
			&end,
			awsservice.AssertLogsNotEmpty(),
			//awsservice.AssertNoDuplicateLogs(),
			awsservice.AssertPerLog(
				awsservice.AssertLogSchema(func(message string) (string, error) {
					var eksClusterType awsservice.EKSClusterType
					innerErr := json.Unmarshal([]byte(message), &eksClusterType)
					if innerErr != nil {
						return "", fmt.Errorf("failed to unmarshal log file: %w", innerErr)
					}

					log.Printf("eksClusterType is: %s", eksClusterType.Type)
					jsonSchema, ok := eks_resources.EksClusterValidationMap[eksClusterType.Type]
					if !ok {
						return "", errors.New("invalid cluster type provided")
					}
					return jsonSchema, nil
				}),
				awsservice.AssertLogContainsSubstring(fmt.Sprintf("\"ClusterName\":\"%s\"", env.EKSClusterName)),
			),
		)

		if err != nil {
			log.Printf("log validation (%s/%s) failed: %v", group, stream, err)
			return testResult
		}
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func ValidateLogsFrequency(env *environment.MetaData) status.TestResult {

	testResult := status.TestResult{
		Name:   "emf-logs-frequency",
		Status: status.FAILED,
	}

	end := time.Now().Add(time.Duration(-2) * time.Minute).Truncate(time.Minute)
	start := end.Add(time.Duration(-1) * time.Minute)
	group := fmt.Sprintf("/aws/containerinsights/%s/performance", env.EKSClusterName)

	// need to get the instances used for the EKS cluster
	eKSInstances, err := awsservice.GetEKSInstances(env.EKSClusterName)
	if err != nil {
		log.Println("failed to get EKS instances", err)
		return testResult
	}

	for _, instance := range eKSInstances {
		stream := *instance.InstanceName
		frequencyMap, err := awsservice.GetLogEventCountPerType(group, stream, &start, &end)

		for logType, expectedFrequency := range eks_resources.EksClusterFrequencyValidationMap {
			log.Printf("logs with no logtype : %d", frequencyMap[awsservice.NoLogTypeFound])

			actualFrequency, ok := frequencyMap[logType]
			if !ok {
				log.Printf("no log with the expected logtype found : %s, start time : %s", logType, start.GoString())
				return testResult
			}
			if actualFrequency != expectedFrequency {
				log.Printf("log frequency validation failed for type: %s, expected: %d, actual: %d, start time: %s", logType, expectedFrequency, actualFrequency, start.GoString())
				return testResult
			}
		}

		if err != nil {
			log.Printf("log validation (%s/%s) failed: %v, start time : %s", group, stream, err, start)
			return testResult
		}
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
