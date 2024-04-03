// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package common

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
		case "app_signals":
			err = SendAppSignalMetrics(metricPerInterval, []string{}, sendingInterval, duration) //does app signals have dimension for metric?
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
func processFile(filePath string, startTime int64) {
	data, err := os.ReadFile(filePath) // Using os.ReadFile here
	if err != nil {
		fmt.Println("Error reading file:", err)
		return
	}

	// Replace START_TIME with the current time
	modifiedData := strings.ReplaceAll(string(data), "START_TIME", fmt.Sprintf("%d", startTime))

	// Mimic `curl` command - sending HTTP POST request
	url := "http://127.0.0.1:4316/v1/metrics"
	_, err = http.Post(url, "application/json", bytes.NewBufferString(modifiedData))
	if err != nil {
		fmt.Println("Error sending request:", err)
	}
}
func SendAppSignalMetrics(metricPerInterval int, metricDimension []string, sendingInterval, duration time.Duration) error {
	// The bash script to be executed asynchronously.
	dir, err := os.Getwd()
	if err != nil {
		fmt.Println("Error getting current directory:", err)
		return err
	}
	fmt.Println("Current Directory:", dir)

	// Determine the base directory for the files based on the OS
	var baseDir string
	if runtime.GOOS == "windows" {
		baseDir = "C:\\Users\\Administrator\\amazon-cloudwatch-agent-test\\test\\app_signals\\resources\\metrics"
	} else { // assuming macOS or Unix-like system
		baseDir = "/Users/ec2-user/amazon-cloudwatch-agent-test/test/app_signals/resources/metrics"
	}

	for i := 0; i < int(duration/5); i++ {
		// Get current time like `date +%s%N`
		startTime := time.Now().UnixNano()

		// Process files with dynamic paths
		processFile(filepath.Join(baseDir, "server_consumer.json"), startTime)
		processFile(filepath.Join(baseDir, "client_producer.json"), startTime)

		// Sleep for 5 seconds
		time.Sleep(5 * time.Second)
	}

	return nil

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
