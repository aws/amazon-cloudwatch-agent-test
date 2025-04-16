// emf_prometheus/common.go
package emf_prometheus

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	namespacePrefix = "emf_prometheus_"
	logGroupPrefix  = "prometheus_test"
)


func setupPrometheus(t *testing.T, prometheusConfig, prometheusMetrics string, jobName string) {
	var configContent string
	if jobName != "" {
		configContent = strings.Replace(prometheusConfig,
			"job_name: 'prometheus_test_job'",
			fmt.Sprintf("job_name: '%s'", jobName),
			1)
	} else {
		configContent = prometheusConfig
	}

	commands := []string{
		fmt.Sprintf("cat <<EOF | sudo tee /tmp/prometheus_config.yaml\n%s\nEOF", configContent),
		fmt.Sprintf("cat <<EOF | sudo tee /tmp/metrics\n%s\nEOF", prometheusMetrics),
		"sudo python3 -m http.server 8101 --directory /tmp &> /dev/null &",
	}

	err := common.RunCommands(commands)
	if err != nil {
		if _, err := common.RunCommand("ls -l /tmp/prometheus_config.yaml"); err != nil {
			log.Printf("prometheus_config.yaml not found: %v", err)
		}
		if _, err := common.RunCommand("ls -l /tmp/metrics"); err != nil {
			log.Printf("metrics file not found: %v", err)
		}
	}
	require.NoError(t, err, "Failed to setup Prometheus")

	if _, err := common.RunCommand("curl -s -f http://localhost:8101/metrics"); err != nil {
		log.Printf("WARNING: HTTP server not responding: %v", err)
	}
}

func startAgent(t *testing.T, agentConfig string, namespace, logGroupName string) {
	// Read the config file
	configContent, err := os.ReadFile(agentConfig)
	require.NoError(t, err, "Failed to read agent config")

	// Replace template values
	configStr := string(configContent)
	configStr = strings.ReplaceAll(configStr, "${NAMESPACE}", namespace)
	configStr = strings.ReplaceAll(configStr, "${LOG_GROUP_NAME}", logGroupName)

	log.Printf("Using namespace: %s and log group: %s", namespace, logGroupName)

	// Write the modified config
	err = os.WriteFile(common.ConfigOutputPath, []byte(configStr), 0644)
	require.NoError(t, err, "Failed to write modified agent config")

	err = common.StartAgent(common.ConfigOutputPath, true, false)
	if err != nil {
		if output, err := common.RunCommand("sudo /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl -a status"); err != nil {
			log.Printf("Agent status check failed: %v\nOutput: %s", err, output)
		}
	}
	require.NoError(t, err)
	time.Sleep(2 * time.Minute)
}


func verifyMetricsInCloudWatch(t *testing.T, namespace string) {
	metricsToCheck := []struct {
		name     string
		promType string
	}{
		{"prometheus_test_counter", "counter"},
		{"prometheus_test_gauge", "gauge"},
		{"prometheus_test_summary_sum", "summary"},
	}

	valueFetcher := metric.MetricValueFetcher{}
	for _, m := range metricsToCheck {
		log.Printf("Checking metric %s of type %s...", m.name, m.promType)

		dims := []types.Dimension{{
			Name:  aws.String("prom_type"),
			Value: aws.String(m.promType),
		}}

		values, err := valueFetcher.Fetch(namespace, m.name, dims, metric.SAMPLE_COUNT, metric.MinuteStatPeriod)
		if err != nil {
			log.Printf("Failed to fetch metric %s: %v", m.name, err)
		}
		require.NoError(t, err, fmt.Sprintf("Failed to fetch metric %s", m.name))

		if len(values) == 0 {
			log.Printf("No values found for metric %s", m.name)
		} else {
			log.Printf("Found %d values for metric %s: %v", len(values), m.name, values)
		}
		require.NotEmpty(t, values, fmt.Sprintf("No values found for metric %s", m.name))
	}
}

func cleanup(t *testing.T, logGroupName string) {
	log.Println("Running cleanup commands...")
	commands := []string{
		"sudo pkill -f 'python3 -m http.server 8101'",
		"sudo rm -f /tmp/prometheus_config.yaml /tmp/metrics",
	}
	err := common.RunCommands(commands)
	if err != nil {
		log.Printf("Cleanup failed: %v", err)
	}
	require.NoError(t, err, "Failed to cleanup")

	log.Printf("Deleting log group: %s", logGroupName)
	awsservice.DeleteLogGroup(logGroupName)

}

func generateRandomSuffix() string {
	return strconv.Itoa(rand.Intn(100000))
}