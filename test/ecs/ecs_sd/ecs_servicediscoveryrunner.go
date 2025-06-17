package ecs_sd

import (
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/stretchr/testify/assert"
	"log"
	"strings"
	"time"
)

const (
	RetryTime = 15
	// Log group format: https://github.com/aws/amazon-cloudwatch-agent/blob/5ef3dba446cb56a4c2306878592b5d14300ae82f/translator/translate/otel/exporter/awsemf/prometheus.go#L38
	ECSLogGroupNameFormat = "/aws/ecs/containerinsights/%s/prometheus"
	// Log stream based on job name in extra_apps.tpl:https://github.com/aws/amazon-cloudwatch-agent-test/blob/main/test/ecs/ecs_sd/resources/extra_apps.tpl#L41
	LogStreamName = "prometheus-redis"

	namespace = "ecs_servicediscovery" //todo
)

type ECSServiceDiscoveryTestRunner struct {
	test_runner.BaseTestRunner
}

func (t ECSServiceDiscoveryTestRunner) GetTestName() string {
	return "ecs_servicediscovery"
}

func (t ECSServiceDiscoveryTestRunner) GetAgentConfigFileName() string {
	return ""
}
func (t ECSServiceDiscoveryTestRunner) GetMeasuredMetrics() []string {
	// dummy function to satisfy the interface
	return []string{}
}

func (t ECSServiceDiscoveryTestRunner) Validate() status.TestGroupResult {
	testResults := []status.TestResult{}
	testResults = append(testResults, t.validateHistogramMetric("my.delta.histogram")...)
	testResults = append(testResults, t.validateHistogramMetric("my.cumulative.histogram")...)

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func TestValidatingCloudWatchLogs(t *testing.T) {
	env := environment.GetEnvironmentMetaData()
	logGroupName, logGroupFound, start, end := ValidateLogGroupFormat(t, env)

	ValidateLogsContent(t, logGroupName, start, end)

	if logGroupFound {
		awsservice.DeleteLogGroupAndStream(logGroupName, LogStreamName)
	}
}

func ValidateLogGroupFormat(t *testing.T, env *environment.MetaData) (string, bool, time.Time, time.Time) {
	start := time.Now()
	logGroupName := fmt.Sprintf(ECSLogGroupNameFormat, env.EcsClusterName)

	var logGroupFound bool
	for currentRetry := 1; ; currentRetry++ {

		if currentRetry == RetryTime {
			t.Fatalf("Test has exhausted %v retry time", RetryTime)
		}

		if !awsservice.IsLogGroupExists(logGroupName) {
			log.Printf("Current retry: %v/%v and begin to sleep for 20s \n", currentRetry, RetryTime)
			time.Sleep(20 * time.Second)
			continue
		}
		break
	}
	end := time.Now()
	return logGroupName, logGroupFound, start, end
}

func ValidateLogsContent(t *testing.T, logGroupName string, start time.Time, end time.Time) {
	err := awsservice.ValidateLogs(
		logGroupName,
		LogStreamName,
		&start,
		&end,
		awsservice.AssertLogsNotEmpty(),
		awsservice.AssertPerLog(
			awsservice.AssertLogSchema(awsservice.WithSchema(schema)),
			func(event types.OutputLogEvent) error {
				if strings.Contains(*event.Message, "CloudWatchMetrics") &&
					!strings.Contains(*event.Message, "\"Namespace\":\"ECS/ContainerInsights/Prometheus\"") {
					return fmt.Errorf("emf log found for non ECS/ContainerInsights/Prometheus namespace: %s", *event.Message)
				}
				return nil
			},
			awsservice.AssertLogContainsSubstring("\"job\":\"prometheus-redis\""),
		),
	)
	assert.NoError(t, err)
}
