// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package emf

import (
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/computetype"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
)

const (
	efaMetricIndicator = "_efa_"

	containerEfaRxBytes                  = "container_efa_rx_bytes"
	containerEfaTxBytes                  = "container_efa_tx_bytes"
	containerEfaRxDropped                = "container_efa_rx_dropped"
	containerEfaRdmaReadBytes            = "container_efa_rdma_read_bytes"
	containerEfaRdmaWriteBytes           = "container_efa_rdma_write_bytes"
	containerEfaRdmaWriteRecvBytes       = "container_efa_rdma_write_recv_bytes"
	containerEfaUnresponsiveRemoteEvents = "container_efa_unresponsive_remote_events"
	containerEfaRetransBytes             = "container_efa_retrans_bytes"
	containerEfaRetransPkts              = "container_efa_retrans_pkts"
	containerEfaImpairedRemoteConnEvents = "container_efa_impaired_remote_conn_events"
	containerEfaRetransTimeoutEvents     = "container_efa_retrans_timeout_events"

	podEfaRxBytes                  = "pod_efa_rx_bytes"
	podEfaTxBytes                  = "pod_efa_tx_bytes"
	podEfaRxDropped                = "pod_efa_rx_dropped"
	podEfaRdmaReadBytes            = "pod_efa_rdma_read_bytes"
	podEfaRdmaWriteBytes           = "pod_efa_rdma_write_bytes"
	podEfaRdmaWriteRecvBytes       = "pod_efa_rdma_write_recv_bytes"
	podEfaUnresponsiveRemoteEvents = "pod_efa_unresponsive_remote_events"
	podEfaRetransBytes             = "pod_efa_retrans_bytes"
	podEfaRetransPkts              = "pod_efa_retrans_pkts"
	podEfaImpairedRemoteConnEvents = "pod_efa_impaired_remote_conn_events"
	podEfaRetransTimeoutEvents     = "pod_efa_retrans_timeout_events"
	podEfaLimit                    = "pod_efa_limit"
	podEfaRequest                  = "pod_efa_request"
	podEfaUsageTotal               = "pod_efa_usage_total"
	podEfaReservedCapacity         = "pod_efa_reserved_capacity"

	nodeEfaRxBytes                  = "node_efa_rx_bytes"
	nodeEfaTxBytes                  = "node_efa_tx_bytes"
	nodeEfaRxDropped                = "node_efa_rx_dropped"
	nodeEfaRdmaReadBytes            = "node_efa_rdma_read_bytes"
	nodeEfaRdmaWriteBytes           = "node_efa_rdma_write_bytes"
	nodeEfaRdmaWriteRecvBytes       = "node_efa_rdma_write_recv_bytes"
	nodeEfaUnresponsiveRemoteEvents = "node_efa_unresponsive_remote_events"
	nodeEfaRetransBytes             = "node_efa_retrans_bytes"
	nodeEfaRetransPkts              = "node_efa_retrans_pkts"
	nodeEfaImpairedRemoteConnEvents = "node_efa_impaired_remote_conn_events"
	nodeEfaRetransTimeoutEvents     = "node_efa_retrans_timeout_events"
	nodeEfaLimit                    = "node_efa_limit"
	nodeEfaReservedCapacity         = "node_efa_reserved_capacity"
	nodeEfaUnreservedCapacity       = "node_efa_unreserved_capacity"
	nodeEfaAvailableCapacity        = "node_efa_available_capacity"
	nodeEfaUsageTotal               = "node_efa_usage_total"
)

var expectedDimsToMetricsIntegTest = map[string][]string{
	"ClusterName": {
		containerEfaRxBytes, containerEfaTxBytes, containerEfaRxDropped, containerEfaRdmaReadBytes, containerEfaRdmaWriteBytes, containerEfaRdmaWriteRecvBytes,
		containerEfaRetransBytes, containerEfaRetransPkts, containerEfaRetransTimeoutEvents, containerEfaImpairedRemoteConnEvents, containerEfaUnresponsiveRemoteEvents,
		podEfaRxBytes, podEfaTxBytes, podEfaRxDropped, podEfaRdmaReadBytes, podEfaRdmaWriteBytes, podEfaRdmaWriteRecvBytes,
		podEfaRetransBytes, podEfaRetransPkts, podEfaRetransTimeoutEvents, podEfaImpairedRemoteConnEvents, podEfaUnresponsiveRemoteEvents,
		podEfaLimit, podEfaRequest, podEfaUsageTotal, podEfaReservedCapacity,
		nodeEfaRxBytes, nodeEfaTxBytes, nodeEfaRxDropped, nodeEfaRdmaReadBytes, nodeEfaRdmaWriteBytes, nodeEfaRdmaWriteRecvBytes,
		nodeEfaRetransBytes, nodeEfaRetransPkts, nodeEfaRetransTimeoutEvents, nodeEfaImpairedRemoteConnEvents, nodeEfaUnresponsiveRemoteEvents,
		nodeEfaLimit, nodeEfaReservedCapacity, nodeEfaUnreservedCapacity, nodeEfaAvailableCapacity, nodeEfaUsageTotal,
	},
	//"ClusterName-Namespace-PodName-ContainerName": {
	//	containerEfaRxBytes, containerEfaTxBytes, containerEfaRxDropped, containerEfaRdmaReadBytes, containerEfaRdmaWriteBytes, containerEfaRdmaWriteRecvBytes,
	//	containerEfaRetransBytes, containerEfaRetransPkts, containerEfaRetransTimeoutEvents, containerEfaImpairedRemoteConnEvents, containerEfaUnresponsiveRemoteEvents,
	//},
	//"ClusterName-Namespace-PodName-FullPodName-ContainerName": {
	//	containerEfaRxBytes, containerEfaTxBytes, containerEfaRxDropped, containerEfaRdmaReadBytes, containerEfaRdmaWriteBytes, containerEfaRdmaWriteRecvBytes,
	//	containerEfaRetransBytes, containerEfaRetransPkts, containerEfaRetransTimeoutEvents, containerEfaImpairedRemoteConnEvents, containerEfaUnresponsiveRemoteEvents,
	//},
	//"ClusterName-Namespace-PodName-FullPodName-ContainerName-NetworkInterfaceId": {
	//	containerEfaRxBytes, containerEfaTxBytes, containerEfaRxDropped, containerEfaRdmaReadBytes, containerEfaRdmaWriteBytes, containerEfaRdmaWriteRecvBytes,
	//	containerEfaRetransBytes, containerEfaRetransPkts, containerEfaRetransTimeoutEvents, containerEfaImpairedRemoteConnEvents, containerEfaUnresponsiveRemoteEvents,
	//},
	"ClusterName-Namespace": {
		podEfaRxBytes, podEfaTxBytes, podEfaRxDropped, podEfaRdmaReadBytes, podEfaRdmaWriteBytes, podEfaRdmaWriteRecvBytes,
		podEfaRetransBytes, podEfaRetransPkts, podEfaRetransTimeoutEvents, podEfaImpairedRemoteConnEvents, podEfaUnresponsiveRemoteEvents,
	},
	//"ClusterName-Namespace-Service": {
	//	podEfaRxBytes, podEfaTxBytes, podEfaRxDropped, podEfaRdmaReadBytes, podEfaRdmaWriteBytes, podEfaRdmaWriteRecvBytes,
	//	podEfaRetransBytes, podEfaRetransPkts, podEfaRetransTimeoutEvents, podEfaImpairedRemoteConnEvents, podEfaUnresponsiveRemoteEvents,
	//},
	"ClusterName-Namespace-PodName": {
		podEfaRxBytes, podEfaTxBytes, podEfaRxDropped, podEfaRdmaReadBytes, podEfaRdmaWriteBytes, podEfaRdmaWriteRecvBytes,
		podEfaRetransBytes, podEfaRetransPkts, podEfaRetransTimeoutEvents, podEfaImpairedRemoteConnEvents, podEfaUnresponsiveRemoteEvents,
		podEfaLimit, podEfaRequest, podEfaUsageTotal, podEfaReservedCapacity,
	},
	//"ClusterName-Namespace-PodName-FullPodName": {
	//	podEfaRxBytes, podEfaTxBytes, podEfaRxDropped, podEfaRdmaReadBytes, podEfaRdmaWriteBytes, podEfaRdmaWriteRecvBytes,
	//	podEfaRetransBytes, podEfaRetransPkts, podEfaRetransTimeoutEvents, podEfaImpairedRemoteConnEvents, podEfaUnresponsiveRemoteEvents,
	//},
	//"ClusterName-Namespace-PodName-FullPodName-NetworkInterfaceId": {
	//	podEfaRxBytes, podEfaTxBytes, podEfaRxDropped, podEfaRdmaReadBytes, podEfaRdmaWriteBytes, podEfaRdmaWriteRecvBytes,
	//	podEfaRetransBytes, podEfaRetransPkts, podEfaRetransTimeoutEvents, podEfaImpairedRemoteConnEvents, podEfaUnresponsiveRemoteEvents,
	//},
	"ClusterName-InstanceId-NodeName": {
		nodeEfaRxBytes, nodeEfaTxBytes, nodeEfaRxDropped, nodeEfaRdmaReadBytes, nodeEfaRdmaWriteBytes, nodeEfaRdmaWriteRecvBytes,
		nodeEfaRetransBytes, nodeEfaRetransPkts, nodeEfaRetransTimeoutEvents, nodeEfaImpairedRemoteConnEvents, nodeEfaUnresponsiveRemoteEvents,
		nodeEfaLimit, nodeEfaReservedCapacity, nodeEfaUnreservedCapacity, nodeEfaAvailableCapacity, nodeEfaUsageTotal,
	},
	"ClusterName-InstanceId-InstanceType-NetworkInterfaceId-NodeName": {
		nodeEfaRxBytes, nodeEfaTxBytes, nodeEfaRxDropped, nodeEfaRdmaReadBytes, nodeEfaRdmaWriteBytes, nodeEfaRdmaWriteRecvBytes,
		nodeEfaRetransBytes, nodeEfaRetransPkts, nodeEfaRetransTimeoutEvents, nodeEfaImpairedRemoteConnEvents, nodeEfaUnresponsiveRemoteEvents,
	},
}

type EfaTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (suite *EfaTestSuite) SetupSuite() {
	fmt.Println(">>>> Starting EFA Container Insights TestSuite")
}

func (suite *EfaTestSuite) TearDownSuite() {
	suite.Result.Print()
	fmt.Println(">>>> Finished EFA Container Insights TestSuite")
}

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

var (
	eksTestRunners []*test_runner.EKSTestRunner
)

func getEksTestRunners(env *environment.MetaData) []*test_runner.EKSTestRunner {
	if eksTestRunners == nil {
		factory := dimension.GetDimensionFactory(*env)

		eksTestRunners = []*test_runner.EKSTestRunner{
			{
				Runner: &EfaTestRunner{test_runner.BaseTestRunner{DimensionFactory: factory}, "EKS_EFA", env},
				Env:    *env,
			},
		}
	}
	return eksTestRunners
}

func (suite *EfaTestSuite) TestAllInSuite() {
	env := environment.GetEnvironmentMetaData()
	switch env.ComputeType {
	case computetype.EKS:
		log.Println("Environment compute type is EKS")
		for _, testRunner := range getEksTestRunners(env) {
			testRunner.Run(suite, env)
		}
	default:
		return
	}

	suite.Assert().Equal(status.SUCCESSFUL, suite.Result.GetStatus(), "EFA Container Test Suite Failed")
}

func (suite *EfaTestSuite) AddToSuiteResult(r status.TestGroupResult) {
	suite.Result.TestGroupResults = append(suite.Result.TestGroupResults, r)
}

func TestEfaSuite(t *testing.T) {
	suite.Run(t, new(EfaTestSuite))
}

type EfaTestRunner struct {
	test_runner.BaseTestRunner
	testName string
	env      *environment.MetaData
}

var _ test_runner.ITestRunner = (*EfaTestRunner)(nil)

func (t *EfaTestRunner) Validate() status.TestGroupResult {
	var testResults []status.TestResult
	expectedDimsToMetrics := expectedDimsToMetricsIntegTest
	testResults = append(testResults, metric.ValidateMetrics(t.env, efaMetricIndicator, expectedDimsToMetrics)...)
	testResults = append(testResults, metric.ValidateLogs(t.env))
	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *EfaTestRunner) GetTestName() string {
	return t.testName
}

func (t *EfaTestRunner) GetAgentConfigFileName() string {
	return ""
}

func (t *EfaTestRunner) GetAgentRunDuration() time.Duration {
	return 3 * time.Minute
}

func (t *EfaTestRunner) GetMeasuredMetrics() []string {
	return nil
}
