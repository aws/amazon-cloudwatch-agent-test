// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/aws-sdk-go-v2/aws"
	"log"
	"net"
	"time"

	"collectd.org/api"
	"collectd.org/exec"
	"collectd.org/network"
	"github.com/DataDog/datadog-go/statsd"
	"github.com/prozz/aws-embedded-metrics-golang/emf"
)

// StartSendingMetrics will generate metrics load based on the receiver (e.g 5000 statsd metrics per minute)
func StartSendingMetrics(receiver string, duration, sendingInterval time.Duration, metricPerInterval int, metricLogGroup, metricNamespace string) (err error) {
	go func() {
		switch receiver {
		case "statsd":
			err = SendStatsdMetrics(metricPerInterval, []string{}, sendingInterval, duration)
		case "collectd":
			err = SendCollectDMetrics(metricPerInterval, sendingInterval, duration)
		case "emf":
			err = SendEMFMetrics(metricPerInterval, metricLogGroup, metricNamespace, sendingInterval, duration)
		default:
		}

	}()

	return err
}

func SendCollectDMetrics(metricPerInterval int, sendingInterval, duration time.Duration) error {
	// https://github.com/collectd/go-collectd/tree/92e86f95efac5eb62fa84acc6033e7a57218b606
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := network.Dial(
		net.JoinHostPort("127.0.0.1", network.DefaultService),
		network.ClientOptions{
			SecurityLevel: network.None,
		})

	if err != nil {
		return err
	}

	defer client.Close()

	ticker := time.NewTicker(sendingInterval)
	defer ticker.Stop()
	endTimeout := time.After(duration)

	// Sending the collectd metric within the first minute before the ticker kicks in the next minute
	for t := 1; t <= metricPerInterval/2; t++ {
		_ = client.Write(ctx, &api.ValueList{
			Identifier: api.Identifier{
				Host:   exec.Hostname(),
				Plugin: fmt.Sprint("gauge_", t),
				Type:   "gauge",
			},
			Time:     time.Now(),
			Interval: time.Minute,
			Values:   []api.Value{api.Gauge(t)},
		})

		err = client.Write(ctx, &api.ValueList{
			Identifier: api.Identifier{
				Host:   exec.Hostname(),
				Plugin: fmt.Sprint("counter_", t),
				Type:   "counter",
			},
			Time:     time.Now(),
			Interval: time.Minute,
			Values:   []api.Value{api.Counter(t)},
		})

		if err != nil && !errors.Is(err, network.ErrNotEnoughSpace) {
			return err
		}
	}

	time.Sleep(30 * time.Second)

	if err := client.Flush(); err != nil {
		return err
	}

	for {
		select {
		case <-ticker.C:
			for t := 1; t <= metricPerInterval/2; t++ {
				_ = client.Write(ctx, &api.ValueList{
					Identifier: api.Identifier{
						Host:   exec.Hostname(),
						Plugin: fmt.Sprint("gauge_", t),
						Type:   "gauge",
					},
					Time:     time.Now(),
					Interval: time.Minute,
					Values:   []api.Value{api.Gauge(t)},
				})

				err = client.Write(ctx, &api.ValueList{
					Identifier: api.Identifier{
						Host:   exec.Hostname(),
						Plugin: fmt.Sprint("counter_", t),
						Type:   "counter",
					},
					Time:     time.Now(),
					Interval: time.Minute,
					Values:   []api.Value{api.Counter(t)},
				})

				if err != nil && !errors.Is(err, network.ErrNotEnoughSpace) {
					return err
				}
			}

			if err := client.Flush(); err != nil {
				return err
			}
		case <-endTimeout:
			return nil
		}
	}

}

func SendStatsdMetrics(metricPerInterval int, metricDimension []string, sendingInterval, duration time.Duration) error {
	// https://github.com/DataDog/datadog-go#metrics
	client, err := statsd.New("127.0.0.1:8125", statsd.WithMaxMessagesPerPayload(100), statsd.WithNamespace("statsd"), statsd.WithoutTelemetry())

	if err != nil {
		return err
	}

	defer client.Close()

	ticker := time.NewTicker(sendingInterval)
	defer ticker.Stop()
	endTimeout := time.After(duration)

	// Sending the statsd metric within the first minute before the ticker kicks in the next minute
	for t := 1; t <= metricPerInterval/2; t++ {
		if err := client.Count(fmt.Sprint("counter_", t), int64(t), metricDimension, 1.0); err != nil {
			return err
		}
		if err := client.Gauge(fmt.Sprint("gauge_", t), float64(t), metricDimension, 1.0); err != nil {
			return err
		}
	}

	for {
		select {
		case <-ticker.C:
			for t := 1; t <= metricPerInterval/2; t++ {
				client.Count(fmt.Sprint("counter_", t), int64(t), metricDimension, 1.0)
				client.Gauge(fmt.Sprint("gauge_", t), float64(t), metricDimension, 1.0)
			}
		case <-endTimeout:
			return nil
		}
	}
}

func SendEMFMetrics(metricPerInterval int, metricLogGroup, metricNamespace string, sendingInterval, duration time.Duration) error {
	// github.com/prozz/aws-embedded-metrics-golang/emf
	conn, err := net.DialTimeout("tcp", "127.0.0.1:25888", time.Millisecond*10000)
	if err != nil {
		return err
	}

	defer conn.Close()

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	endTimeout := time.After(duration)

	for t := 1; t <= metricPerInterval; t++ {
		emf.New(emf.WithWriter(conn), emf.WithLogGroup(metricLogGroup)).
			Namespace(metricNamespace).
			DimensionSet(
				emf.NewDimension("InstanceId", metricLogGroup),
			).
			MetricAs(fmt.Sprint("emf_time_", t), t, emf.Milliseconds).
			Log()

	}

	for {
		select {
		case <-ticker.C:
			for t := 1; t <= metricPerInterval; t++ {
				emf.New(emf.WithWriter(conn), emf.WithLogGroup(metricLogGroup)).
					Namespace(metricNamespace).
					DimensionSet(
						emf.NewDimension("InstanceId", metricLogGroup),
					).
					MetricAs(fmt.Sprint("emf_time_", t), t, emf.Milliseconds).
					Log()

			}
		case <-endTimeout:
			return nil
		}
	}
}

func ValidateStatsdMetric(dimFactory dimension.Factory, namespace string, dimensionKey string, metricName string, runDuration time.Duration) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}
	instructions := []dimension.Instruction{
		{
			Key:   dimensionKey,
			Value: dimension.UnknownDimensionValue(),
		},
		{
			Key:   "key",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("value")},
		},
	}
	switch metricName {
	case "statsd_counter_1":
		instructions = append(instructions, dimension.Instruction{
			// CWA adds this metric_type dimension.
			Key:   "metric_type",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("counter")},
		})
	case "statsd_gauge_1":
		instructions = append(instructions, dimension.Instruction{
			// CWA adds this metric_type dimension.
			Key:   "metric_type",
			Value: dimension.ExpectedDimensionValue{Value: aws.String("gauge")},
		})
	}

	dims, failed := dimFactory.GetDimensions(instructions)
	if len(failed) > 0 {
		return testResult
	}
	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(namespace, metricName, dims, metric.AVERAGE, 30)
	if err != nil {
		return testResult
	}

	aggregationInterval := 30 * time.Second
	upperBound := int(runDuration/aggregationInterval) + 2
	lowerBound := int(runDuration/aggregationInterval) - 4

	if len(values) < lowerBound || len(values) > upperBound {
		log.Printf("fail: lowerBound %v, upperBound %v, actual %v",
			lowerBound, upperBound, len(values))
		return testResult
	}

	switch metricName {
	case "statsd_counter_1":
		if !isAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 4) {
			return testResult
		}
	case "statsd_gauge_1":
		if !isAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 1) {
			return testResult
		}
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func isAllValuesGreaterThanOrEqualToExpectedValue(metricName string, values []float64, expectedValue float64) bool {
	if len(values) == 0 {
		log.Printf("No values found %v", metricName)
		return false
	}

	totalSum := 0.0
	for _, value := range values {
		if value < 0 {
			log.Printf("Values are not all greater than or equal to zero for %s", metricName)
			return false
		}
		totalSum += value
	}
	metricErrorBound := 0.2
	metricAverageValue := totalSum / float64(len(values))
	upperBoundValue := expectedValue * (1 + metricErrorBound)
	lowerBoundValue := expectedValue * (1 - metricErrorBound)
	if expectedValue > 0 && (metricAverageValue > upperBoundValue || metricAverageValue < lowerBoundValue) {
		log.Printf("The average value %f for metric %s are not within bound [%f, %f]", metricAverageValue, metricName, lowerBoundValue, upperBoundValue)
		return false
	}

	log.Printf("The average value %f for metric %s are within bound [%f, %f]", expectedValue, metricName, lowerBoundValue, upperBoundValue)
	return true
}
