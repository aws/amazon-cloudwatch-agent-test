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
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/aws-sdk-go/aws"
	"log"
	"net"
	"net/http"
	"os"
	exec2 "os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"collectd.org/api"
	"collectd.org/exec"
	"collectd.org/network"
	"github.com/DataDog/datadog-go/statsd"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/prozz/aws-embedded-metrics-golang/emf"
)

const SleepDuration = 5 * time.Second

const TracesEndpoint = "4316/v1/traces"
const MetricEndpoint = "4316/v1/metrics"
const TMPAGENTPATH = "/tmp/agent_config.json"

//go:embed prometheus/prometheus.yaml
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
	log.Println("metricPerInterval is here: ", metricPerInterval)
	log.Println("metricPerInterval is here: ", sendingInterval)
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
				//ScrapeInterval: int(sendingInterval.Seconds()),
				ScrapeInterval: 10,
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

type CloudWatchMetricLog struct {
	CloudWatchMetrics []struct {
		Namespace  string     `json:"Namespace"`
		Dimensions [][]string `json:"Dimensions"`
		Metrics    []struct {
			Name              string `json:"Name"`
			Unit              string `json:"Unit"`
			StorageResolution int    `json:"StorageResolution"`
		} `json:"Metrics"`
	} `json:"CloudWatchMetrics"`
}

func countMetricsInLogs(logGroupName string) (int, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion("us-west-2"),
	)
	if err != nil {
		return 0, fmt.Errorf("unable to load SDK config: %v", err)
	}

	client := cloudwatchlogs.NewFromConfig(cfg)

	// Get most recent log stream
	streamInput := &cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName: aws.String(logGroupName),
		OrderBy:      types.OrderByLastEventTime,
		Descending:   aws.Bool(true),
		Limit:        aws.Int32(1),
	}

	streams, err := client.DescribeLogStreams(context.TODO(), streamInput)
	if err != nil {
		return 0, fmt.Errorf("failed to get log streams: %v", err)
	}

	if len(streams.LogStreams) == 0 {
		return 0, fmt.Errorf("no log streams found")
	}

	totalMetrics := 0
	var nextToken *string
	eventCount := 0

	for {
		input := &cloudwatchlogs.GetLogEventsInput{
			LogGroupName:  aws.String(logGroupName),
			LogStreamName: streams.LogStreams[0].LogStreamName,
			StartFromHead: aws.Bool(true),
			NextToken:     nextToken,
			Limit:         aws.Int32(10000),
		}

		resp, err := client.GetLogEvents(context.TODO(), input)
		if err != nil {
			return totalMetrics, fmt.Errorf("failed to get log events: %v", err)
		}

		batchMetrics := 0
		// Process events in this batch
		for _, event := range resp.Events {
			var metricLog CloudWatchMetricLog
			if err := json.Unmarshal([]byte(*event.Message), &metricLog); err != nil {
				log.Printf("Failed to parse log event: %v", err)
				continue
			}

			for _, cwMetric := range metricLog.CloudWatchMetrics {
				batchMetrics += len(cwMetric.Metrics)
			}
			eventCount++
		}

		totalMetrics += batchMetrics
		log.Printf("Processed batch of %d events with %d metrics, running total: %d metrics",
			len(resp.Events), batchMetrics, totalMetrics)

		// Check if we've processed all events
		if resp.NextForwardToken == nil || (nextToken != nil && *resp.NextForwardToken == *nextToken) {
			break
		}
		nextToken = resp.NextForwardToken
	}

	log.Printf("Finished processing %d events with total of %d metrics", eventCount, totalMetrics)
	return totalMetrics, nil
}

func readAgentLogs() (string, error) {
	cmd := exec2.Command("sudo", "cat", "/opt/aws/amazon-cloudwatch-agent/logs/amazon-cloudwatch-agent.log")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to read agent logs: %v", err)
	}
	return string(output), nil
}

func SendPrometheusMetrics(config PrometheusConfig, duration time.Duration) error {
	cleanupPortPrometheus(config.Port)

	// Get Avalanche parameters based on desired metric count
	counter, gauge, summary, series, label := getAvalancheParams(config.MetricCount)
	log.Printf("[Prometheus] Using parameters: counter=%d, gauge=%d, summary=%d, series=%d, label=%d",
		counter, gauge, summary, series, label)

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

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Avalanche: %v", err)
	}

	defer cleanupPortPrometheus(config.Port)

	log.Printf("[Prometheus] Avalanche started on port %d", config.Port)

	// Wait for Avalanche to start up
	time.Sleep(5 * time.Second)

	// Check if Avalanche is running by curling the metrics endpoint
	curlCmd := exec2.Command("curl", "-s", fmt.Sprintf("http://localhost:%d/metrics", config.Port))
	output, err := curlCmd.Output()
	if err != nil {
		return fmt.Errorf("Avalanche failed to start: %v", err)
	}
	log.Printf("[Prometheus] Avalanche is running successfully. Sample output:\n%s", string(output[:200]))

	// Create Prometheus config
	if err := createPrometheusConfig(config.ScrapeInterval); err != nil {
		return fmt.Errorf("failed to create Prometheus config: %v", err)
	}

	log.Printf("[Prometheus] Successfully created Prometheus config at /tmp/prometheus.yaml")

	namespace := updateAgentConfigNamespacePrometheus(TMPAGENTPATH, config.InstanceID)

	agentCmd := exec2.Command("sudo", "/opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl",
		"-a", "fetch-config",
		"-s",
		"-m", "ec2",
		"-c", "file:/tmp/agent_config.json")

	if err := agentCmd.Run(); err != nil {
		return fmt.Errorf("failed to start CloudWatch agent: %v", err)
	}

	log.Printf("[Prometheus] Running for duration: %v", duration)
	time.Sleep(duration)

	log.Println("counting metrics in logs")
	count, err := countMetricsInLogs(namespace)
	if err != nil {
		return fmt.Errorf("error counting metrics: %v", err)
	}

	log.Printf("Found %d metrics in loggroup %s", count, namespace)

	if count < config.MetricCount {
		return fmt.Errorf("insufficient metrics generated: expected ~%d, got %d", config.MetricCount, count)
	}

	agentLogs, err := readAgentLogs()
	if err != nil {
		log.Printf("Warning: Failed to read agent logs: %v", err)
	} else {
		log.Printf("Agent Logs:\n%s", agentLogs)
	}

	log.Printf("[Prometheus] Completed running for specified duration")
	return nil
}

func cleanupPortPrometheus(port int) {
	killCmd := exec2.Command("sudo", "fuser", "-k", fmt.Sprintf("%d/tcp", port))
	if err := killCmd.Run(); err != nil {
		log.Printf("[Prometheus] Failed to kill port %d: %v", port, err)
	}
}

func getAvalancheParams(metricPerInterval int) (counter, gauge, summary, series, label int) {
	switch metricPerInterval {
	case 1000:
		return 100, 100, 20, 10, 0

	case 5000:
		return 100, 100, 20, 50, 0

	case 10000:
		return 100, 100, 20, 100, 10

	case 50000:
		return 100, 500, 20, 500, 10

	default:
		return 10, 10, 5, 20, 10
	}
}

func createPrometheusConfig(scrapeInterval int) error {
	log.Printf("[Prometheus] Creating config with scrape interval: %ds", scrapeInterval)

	cfg := prometheusTemplate
	cfg = strings.ReplaceAll(cfg, "$SCRAPE_INTERVAL", fmt.Sprintf("%ds", scrapeInterval))
	cfg = strings.ReplaceAll(cfg, "$PORT", fmt.Sprintf("%d", 8101))

	log.Printf("[Prometheus] Writing config to /tmp/prometheus.yaml:\n%s", cfg)

	err := os.WriteFile("/tmp/prometheus.yaml", []byte(cfg), os.ModePerm)
	if err != nil {
		log.Printf("[Prometheus] Failed to write config: %v", err)
		return err
	}

	log.Printf("[Prometheus] Successfully wrote config file")
	return nil
}

// Behavior:
// 1. On the first run:
//   - Replaces "CloudWatchAgentStress/Prometheus".
//   - Replaces it with "CloudWatchAgentStress/Prometheus/{instanceID}/1".
//
// 2. On subsequent runs (if stress test retried):
//   - Detects an existing namespace in the form "CloudWatchAgentStress/Prometheus/{instanceID}/{index}".
//   - Increments the {index} by 1 and replaces it with the new value.
//
// The function writes the updated config back to the file and returns the new namespace used.
func updateAgentConfigNamespacePrometheus(configPath string, instanceID string) string {
	// Read the agent config file
	fmt.Printf("Reading config file from: %s\n", configPath)
	data, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Printf("failed to read agent config: %v\n", err)
		os.Exit(1)
	}

	cfg := string(data)
	fmt.Printf("Original config content:\n%s\n", cfg)

	// Match full namespace with instanceID and index: CloudWatchAgentStress/Prometheus/{instanceID}/{index}
	fullPattern := fmt.Sprintf(`CloudWatchAgentStress/Prometheus/%s/(\d+)`, regexp.QuoteMeta(instanceID))
	fmt.Printf("Looking for pattern: %s\n", fullPattern)
	fullRegex := regexp.MustCompile(fullPattern)

	var newNamespace string

	if matches := fullRegex.FindStringSubmatch(cfg); len(matches) == 2 {
		// Found existing namespace with index
		oldIndex, _ := strconv.Atoi(matches[1])
		newIndex := oldIndex + 1
		newNamespace = fmt.Sprintf("CloudWatchAgentStress/Prometheus/%s/%d", instanceID, newIndex)
		fmt.Printf("Found existing namespace. Updating from index %d to %d\n", oldIndex, newIndex)
		fmt.Printf("Old namespace: %s\n", matches[0])
		fmt.Printf("New namespace: %s\n", newNamespace)
		cfg = fullRegex.ReplaceAllString(cfg, newNamespace)
	} else {
		// First-time update: replace base namespace
		newNamespace = fmt.Sprintf("CloudWatchAgentStress/Prometheus/%s/1", instanceID)
		fmt.Printf("No existing namespace found. Creating first-time namespace: %s\n", newNamespace)
		baseRegex := regexp.MustCompile(`CloudWatchAgentStress/Prometheus`)
		cfg = baseRegex.ReplaceAllString(cfg, newNamespace)
	}

	fmt.Printf("Updated config content:\n%s\n", cfg)

	// Write updated config
	fmt.Printf("Writing updated config back to: %s\n", configPath)
	if err := os.WriteFile(configPath, []byte(cfg), os.ModePerm); err != nil {
		fmt.Printf("failed to write modified config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Successfully updated config file with new namespace: %s\n", newNamespace)

	return newNamespace
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
