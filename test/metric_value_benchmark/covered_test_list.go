// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux && integration
// +build linux,integration

package metric_value_benchmark

type TestCoverageStatus string

const (
	TEST_EXISTS         TestCoverageStatus = "TestExists"
	NEED_TEST           TestCoverageStatus = "NeedTest"
	TESTED_INTERNALLY   TestCoverageStatus = "TestedInternally"
	UNSUPPORTED_FEATURE TestCoverageStatus = "UnsupportedFeature"
)

type SetupTypeTestCoverage struct {
	EcsDaemon  TestCoverageStatus
	EcsSidecar TestCoverageStatus
	EcsReplica TestCoverageStatus
	Ec2        TestCoverageStatus
}

const HostMetricCommonCoverage = SetupTypeTestCoverage{EcsDaemon: TEST_EXISTS, EcsSidecar: TEST_EXISTS, EcsReplica: UNSUPPORTED_FEATURE, Ec2: TEST_EXISTS}

var supportedMetricsToTestCoverage = map[string]SetupTypeTestCoverage{
	"cpu_time_active":      HostMetricCommonCoverage,
	"cpu_time_guest":       HostMetricCommonCoverage,
	"cpu_time_guest_nice":  HostMetricCommonCoverage,
	"cpu_time_idle":        HostMetricCommonCoverage,
	"cpu_time_iowait":      HostMetricCommonCoverage,
	"cpu_time_irq":         HostMetricCommonCoverage,
	"cpu_time_nice":        HostMetricCommonCoverage,
	"cpu_time_softirq":     HostMetricCommonCoverage,
	"cpu_time_steal":       HostMetricCommonCoverage,
	"cpu_time_system":      HostMetricCommonCoverage,
	"cpu_time_user":        HostMetricCommonCoverage,
	"cpu_usage_active":     HostMetricCommonCoverage,
	"cpu_usage_guest":      HostMetricCommonCoverage,
	"cpu_usage_guest_nice": HostMetricCommonCoverage,
	"cpu_usage_idle":       HostMetricCommonCoverage,
	"cpu_usage_iowait":     HostMetricCommonCoverage,
	"cpu_usage_irq":        HostMetricCommonCoverage,
	"cpu_usage_nice":       HostMetricCommonCoverage,
	"cpu_usage_softirq":    HostMetricCommonCoverage,
	"cpu_usage_steal":      HostMetricCommonCoverage,
	"cpu_usage_system":     HostMetricCommonCoverage,
	"cpu_usage_user":       HostMetricCommonCoverage,

	"mem_active":            HostMetricCommonCoverage,
	"mem_available":         HostMetricCommonCoverage,
	"mem_available_percent": HostMetricCommonCoverage,
	"mem_buffered":          HostMetricCommonCoverage,
	"mem_cached":            HostMetricCommonCoverage,
	"mem_free":              HostMetricCommonCoverage,
	"mem_inactive":          HostMetricCommonCoverage,
	"mem_total":             HostMetricCommonCoverage,
	"mem_used":              HostMetricCommonCoverage,
	"mem_used_percent":      HostMetricCommonCoverage,
}

func getEcsDaemonMetricsToTest() string[] {
	metricsToTest := []string{}
	for metricName, setupTypeTestCoverage := range supportedMetricsToTestCoverage {
		if (setupTypeTestCoverage.EcsDaemon == TestCoverageStatus.TEST_EXISTS) {
			metricsToTest = append(metricsToTest, metricName)
		}
	}
	return metricsToTest
}
