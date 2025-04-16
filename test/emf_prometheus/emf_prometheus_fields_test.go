package emf_prometheus

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

//go:embed resources/fields/prometheus.yaml
var fieldsPrometheusConfig string

//go:embed resources/fields/prometheus_metrics
var fieldsPrometheusMetrics string

func TestPrometheusEMFFields(t *testing.T) {
	randomSuffix := generateRandomSuffix()
	namespace := fmt.Sprintf("%s_fields_test_%s", namespacePrefix, randomSuffix)
	logGroupName := fmt.Sprintf("%s_fields_test_%s", logGroupPrefix, randomSuffix)

	log.Printf("Starting EMF fields test with namespace: %s and log group: %s",
		namespace, logGroupName)

	setupPrometheus(t, fieldsPrometheusConfig, fieldsPrometheusMetrics, "")
	startAgent(t,
		filepath.Join("agent_configs", "emf_prometheus_config.json"),
		namespace,
		logGroupName)
	verifyEMFFields(t, logGroupName)
	cleanup(t, logGroupName)
}

func verifyEMFFields(t *testing.T, logGroupName string) {
	log.Printf("Verifying EMF fields in log group %s...", logGroupName)

	streams := awsservice.GetLogStreams(logGroupName)
	require.NotEmpty(t, streams, "No log streams found in log group %s", logGroupName)

	since := time.Now().Add(-5 * time.Minute)
	until := time.Now()

	events, err := awsservice.GetLogsSince(logGroupName, *streams[0].LogStreamName, &since, &until)
	require.NoError(t, err, "Failed to get log events")
	require.NotEmpty(t, events, "No log events found")

	requiredFields := []string{
		"host",
		"instance",
		"job",
		"prom_metric_type",
	}

	foundValidEMF := false
	for _, event := range events {
		if !strings.Contains(*event.Message, `"CloudWatchMetrics"`) {
			continue
		}

		var emfLog map[string]interface{}
		err := json.Unmarshal([]byte(*event.Message), &emfLog)
		if err != nil {
			log.Printf("Failed to parse EMF log: %v", err)
			continue
		}

		log.Printf("Checking EMF log: %s", *event.Message)

		missingFields := []string{}
		for _, field := range requiredFields {
			if _, ok := emfLog[field]; !ok {
				missingFields = append(missingFields, field)
			}
		}

		if len(missingFields) > 0 {
			log.Printf("EMF log missing fields: %v", missingFields)
			continue
		}

		host, _ := emfLog["host"].(string)
		require.True(t, strings.Contains(host, ".internal") || strings.Contains(host, ".amazonaws.com"),
			"Host field doesn't match expected format: %s", host)

		instance, _ := emfLog["instance"].(string)
		require.Equal(t, "localhost:8101", instance,
			"Instance field doesn't match expected value: %s", instance)

		job, _ := emfLog["job"].(string)
		require.Equal(t, "prometheus_test_job", job,
			"Job field doesn't match expected value: %s", job)

		metricType, _ := emfLog["prom_metric_type"].(string)
		require.Contains(t, []string{"counter", "gauge", "summary"}, metricType,
			"Unexpected metric type: %s", metricType)

		foundValidEMF = true
		log.Printf("Found valid EMF log with all required fields")
		break
	}

	require.True(t, foundValidEMF, "No valid EMF logs found with all required fields")
}
