// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package common

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	exec2 "os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"collectd.org/api"
	"collectd.org/exec"
	"collectd.org/network"
	"github.com/DataDog/datadog-go/statsd"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/prozz/aws-embedded-metrics-golang/emf"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common/prometheus_helper"
)

const SleepDuration = 5 * time.Second

const TracesEndpoint = "4316/v1/traces"
const MetricEndpoint = "4316/v1/metrics"
const TMPAGENTPATH = "/tmp/agent_config.json"

//go:embed prometheus_helper/prometheus.yaml
var prometheusTemplate string

type PrometheusConfig struct {
	MetricCount    int           `json:"metric_count"`
	Port           int           `json:"port"`
	UpdateInterval time.Duration `json:"update_interval"`
	ScrapeInterval int           `json:"scrape_interval"`
	InstanceID     string        `json:"instance_id"`
}

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
			err = SendAppSignalMetrics(duration) //does app signals have dimension for metric?
		case "prometheus":
			cfg := PrometheusConfig{
				MetricCount:    metricPerInterval,
				Port:           8101,
				UpdateInterval: sendingInterval,
				ScrapeInterval: int(sendingInterval.Seconds()),
				InstanceID:     metricLogGroup,
			}
			err = SendPrometheusMetrics(cfg, duration)
		case "traces":
			err = SendAppSignalsTraceMetrics(duration) //does app signals have dimension for metric?

		default:
		}
	}()

	return err
}

func SendAppSignalsTraceMetrics(duration time.Duration) error {
	baseDir := getBaseDir()

	for i := 0; i < int(duration/(5*time.Second)); i++ {
		startTime := time.Now().UnixNano()
		traceID := generateTraceID()
		traceIDStr := hex.EncodeToString(traceID[:])

		err := processTraceFile(filepath.Join(baseDir, "traces.json"), startTime, traceIDStr)
		if err != nil {
			fmt.Println("Error processing trace file:", err)
			return err
		}

		time.Sleep(5 * time.Second)
	}

	return nil
}

func SendPrometheusMetrics(config PrometheusConfig, agentCollectionDuration time.Duration) error {
	prometheus_helper.CleanupPortPrometheus(config.Port)

	counter, gauge, summary, series, label := prometheus_helper.GetAvalancheParams(config.MetricCount)

	gopathCmd := exec2.Command("go", "env", "GOPATH")
	gopathBytes, err := gopathCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get GOPATH: %v", err)
	}
	gopath := strings.TrimSpace(string(gopathBytes))

	cmd := exec2.Command("sudo", filepath.Join(gopath, "bin", "avalanche"),
		fmt.Sprintf("--port=%d", config.Port),
		fmt.Sprintf("--counter-metric-count=%d", counter),
		fmt.Sprintf("--gauge-metric-count=%d", gauge),
		fmt.Sprintf("--summary-metric-count=%d", summary),
		fmt.Sprintf("--series-count=%d", series),
		fmt.Sprintf("--label-count=%d", label),
		fmt.Sprintf("--const-label=InstanceId=%s", config.InstanceID),
		"--series-change-interval=0",
		"--series-interval=0",
		"--value-interval=10")

	time.Sleep(5 * time.Minute)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Avalanche: %v", err)
	}

	defer prometheus_helper.CleanupPortPrometheus(config.Port)

	curlCmd := exec2.Command("curl", "-s", fmt.Sprintf("http://localhost:%d/metrics", config.Port))
	err = curlCmd.Run()
	if err != nil {
		return fmt.Errorf("Avalanche failed to start: %v", err)
	}

	if err := prometheus_helper.CreatePrometheusConfig(prometheusTemplate, config.ScrapeInterval); err != nil {
		return fmt.Errorf("failed to create Prometheus config: %v", err)
	}
	//namespace and log group are the same
	namespaceAndLogGroup := prometheus_helper.UpdateNamespace(TMPAGENTPATH, config.InstanceID)

	//Restarting agent with updated namespace and log group
	agentCmd := exec2.Command("sudo", "/opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl",
		"-a", "fetch-config",
		"-s",
		"-m", "ec2",
		"-c", "file:/tmp/agent_config.json")

	if err := agentCmd.Run(); err != nil {
		return fmt.Errorf("failed to start CloudWatch agent: %v", err)
	}

	time.Sleep(agentCollectionDuration)
	count, err := awsservice.CountMetricsInEMFLogs(namespaceAndLogGroup)

	if err != nil {
		return fmt.Errorf("E! error counting metrics: %v", err)
	}

	// This is just to generally verify that we are getting metrics in the emf logs
	if count < config.MetricCount {
		return fmt.Errorf("insufficient metrics generated: expected ~%d, got %d", config.MetricCount, count)
	}
	awsservice.DeleteLogGroup(namespaceAndLogGroup)

	return nil
}

func getBaseDir() string {
	if runtime.GOOS == "windows" {
		return "C:\\Users\\Administrator\\amazon-cloudwatch-agent-test\\test\\app_signals\\resources\\traces"
	}
	return "/Users/ec2-user/amazon-cloudwatch-agent-test/test/app_signals/resources/traces"
}

func generateTraceID() [16]byte {
	var r [16]byte
	epochNow := time.Now().Unix()
	binary.BigEndian.PutUint32(r[0:4], uint32(epochNow))
	rand.Read(r[4:])
	return r
}

func processTraceFile(filePath string, startTime int64, traceID string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	modifiedData := strings.ReplaceAll(string(data), "START_TIME", fmt.Sprintf("%d", startTime))
	modifiedData = strings.ReplaceAll(modifiedData, "TRACE_ID", traceID)

	url := "http://127.0.0.1:" + TracesEndpoint
	_, err = http.Post(url, "application/json", bytes.NewBufferString(modifiedData))
	if err != nil {
		return err
	}

	return nil
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
func processFile(filePath string, startTime int64) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Println("Error reading file:", err)
		return nil
	}

	//replace START_TIME with the current time
	modifiedData := strings.ReplaceAll(string(data), "START_TIME", fmt.Sprintf("%d", startTime))

	//curl command
	url := "http://127.0.0.1:" + MetricEndpoint
	_, err = http.Post(url, "application/json", bytes.NewBufferString(modifiedData))

	_, err = http.Post(url, "application/json", bytes.NewBufferString(modifiedData))
	if err != nil {
		fmt.Println("Failed to send POST request to", url)
		fmt.Printf("Error: %v\n", err)
		return err
	}
	return nil

}

func SendAppSignalMetrics(duration time.Duration) error {
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
		baseDir = filepath.Join("C:", "Users", "Administrator", "amazon-cloudwatch-agent-test", "test", "app_signals", "resources", "metrics")
	} else { // assuming macOS or Unix-like system
		baseDir = filepath.Join("/", "Users", "ec2-user", "amazon-cloudwatch-agent-test", "test", "app_signals", "resources", "metrics")
	}

	fmt.Println("Base directory:", baseDir)

	for i := 0; i < int(duration/SleepDuration); i++ {
		if err != nil {
			return err
		}

		//start time to send to process file
		startTime := time.Now().UnixNano()

		//process files
		err = processFile(filepath.Join(baseDir, "server_consumer.json"), startTime)
		if err != nil {
			return err
		}
		err = processFile(filepath.Join(baseDir, "client_producer.json"), startTime)
		if err != nil {
			return err
		}

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

// This function builds and signs an ListEntitiesForMetric call, essentially trying to replicate this curl command:
//
//	curl -i -X POST monitoring.us-west-2.amazonaws.com -H 'Content-Type: application/json' \
//	  -H 'Content-Encoding: amz-1.0' \
//	  --user "$AWS_ACCESS_KEY_ID:$AWS_SECRET_ACCESS_KEY" \
//	  -H "x-amz-security-token: $AWS_SESSION_TOKEN" \
//	  --aws-sigv4 "aws:amz:us-west-2:monitoring" \
//	  -H 'X-Amz-Target: com.amazonaws.cloudwatch.v2013_01_16.CloudWatchVersion20130116.ListEntitiesForMetric' \
//	  -d '{
//		   // sample request body:
//	    "Namespace": "CWAgent",
//	    "MetricName": "cpu_usage_idle",
//	    "Dimensions": [{"Name": "InstanceId", "Value": "i-0123456789012"}, { "Name": "cpu", "Value": "cpu-total"}]
//	  }'
func BuildListEntitiesForMetricRequest(body []byte, region string) (*http.Request, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	if err != nil {
		return nil, err
	}
	signer := v4.NewSigner()
	h := sha256.New()

	h.Write(body)
	payloadHash := hex.EncodeToString(h.Sum(nil))

	// build the request
	req, err := http.NewRequest("POST", "https://monitoring."+region+".amazonaws.com/", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	// set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Amz-Target", "com.amazonaws.cloudwatch.v2013_01_16.CloudWatchVersion20130116.ListEntitiesForMetric")
	req.Header.Set("Content-Encoding", "amz-1.0")

	// set creds
	credentials, err := cfg.Credentials.Retrieve(context.TODO())
	if err != nil {
		return nil, err
	}

	req.Header.Set("x-amz-security-token", credentials.SessionToken)

	// sign the request
	err = signer.SignHTTP(context.TODO(), credentials, req, payloadHash, "monitoring", region, time.Now())
	if err != nil {
		return nil, err
	}

	return req, nil
}
