// emf_prometheus/untyped_test.go
package emf_prometheus

import (
	_ "embed"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

//go:embed resources/untyped/prometheus.yaml
var untypedPrometheusConfig string

//go:embed resources/untyped/prometheus_metrics
var untypedPrometheusMetrics string

func TestPrometheusEMF(t *testing.T) {
	randomSuffix := generateRandomSuffix()
	namespace:=    fmt.Sprintf("%s_untyped_test_%s", namespacePrefix, randomSuffix)
	logGroupName := fmt.Sprintf("%s_untyped_test_%s", logGroupPrefix, randomSuffix)

	log.Printf("Starting untyped metrics test with namespace: %s and log group: %s", namespace, logGroupName)

	setupPrometheus(t, untypedPrometheusConfig, untypedPrometheusMetrics, "")
	startAgent(t,
		filepath.Join("agent_configs", "emf_prometheus_untyped_config.json"),
		namespace,
		logGroupName)
	verifyUntypedMetricAbsence(t, namespace)
	verifyUntypedMetricLogsAbsence(t, logGroupName)
	verifyMetricsInCloudWatch(t, namespace)
	cleanup(t, logGroupName)
}

func verifyUntypedMetricAbsence(t *testing.T, namespace string) {
	dims := []types.Dimension{
		{
			Name:  aws.String("prom_type"),
			Value: aws.String("untyped"),
		},
	}

	valueFetcher := metric.MetricValueFetcher{}
	values, err := valueFetcher.Fetch(namespace, "prometheus_test_untyped", dims, metric.SAMPLE_COUNT, metric.MinuteStatPeriod)

	if err != nil {
		log.Printf("Error fetching untyped metric: %v", err)
	}

	require.Empty(t, values, "Untyped metric was found when it should have been filtered out")
}

func verifyUntypedMetricLogsAbsence(t *testing.T, logGroupName string) {
	log.Printf("Checking CloudWatch Logs for absence of untyped metric logs...")

	streams := awsservice.GetLogStreams(logGroupName)
	require.NotEmpty(t, streams, "No log streams found in log group %s", logGroupName)

	since := time.Now().Add(-5 * time.Minute)
	until := time.Now()

	events, err := awsservice.GetLogsSince(logGroupName, *streams[0].LogStreamName, &since, &until)
	require.NoError(t, err, "Failed to get log events")

	log.Printf("Checking logs in stream %s for untyped metrics...", *streams[0].LogStreamName)
	for _, event := range events {
		if strings.Contains(*event.Message, "prometheus_test_untyped") {
			log.Printf("Found untyped metric in log: %s", *event.Message)
			t.Fatalf("untyped metric found in logs when it should have been filtered")
		}
	}

	log.Printf("Verifying presence of other metric types in logs...")
	metricsToCheck := []string{
		"prometheus_test_counter",
		"prometheus_test_gauge",
		"prometheus_test_summary",
	}

	for _, metricName := range metricsToCheck {
		found := false
		for _, event := range events {
			if strings.Contains(*event.Message, metricName) {
				found = true
				log.Printf("Found metric %s in logs", metricName)
				break
			}
		}
		require.True(t, found, "Failed to find metric %s in logs", metricName)
	}
}