// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package emf_prometheus

import (
	_ "embed"
	"fmt"
	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"log"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

//go:embed resources/token_replacement/prometheus.yaml
var prometheusConfig string

//go:embed resources/token_replacement/prometheus_metrics
var prometheusMetrics string

const (
	prometheusNamespace = "PrometheusEMFJobTest"
	jobNamePrefix       = "prometheus"
)

var jobName string

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestPrometheusEMFTokenReplacement(t *testing.T) {
	randomSuffix := generateRandomSuffix()
	jobName = fmt.Sprintf("%s_tr_test_%s", jobNamePrefix, randomSuffix)
	namespace := fmt.Sprintf("%s_tr_test_%s", namespacePrefix, randomSuffix)


	log.Printf("Starting token replacement test with namespace: %s and job name: %s",
		namespace, jobName)

	setupPrometheus(t, prometheusConfig, prometheusMetrics, jobName)
	startAgent(t,
		filepath.Join("agent_configs", "emf_prometheus_tr_config.json"),
		namespace,
		"")
	verifyLogGroupExists(t, jobName)
	verifyMetricsInCloudWatch(t, namespace)
	cleanup(t, jobName)
}

func verifyLogGroupExists(t *testing.T, logGroupName string) {
	log.Printf("Checking for existence of log group %s...", logGroupName)

	maxAttempts := 10
	for i := 0; i < maxAttempts; i++ {
		if awsservice.IsLogGroupExists(logGroupName) {
			log.Printf("Log group %s exists", logGroupName)
			return
		}
		log.Printf("Log group %s not found, attempt %d/%d, waiting...", logGroupName, i+1, maxAttempts)
		time.Sleep(30 * time.Second)
	}

	require.Fail(t, fmt.Sprintf("Log group %s was not created after %d attempts", logGroupName, maxAttempts))
}
