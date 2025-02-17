// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"log"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const jmxNamespace = "MetricValueBenchmarkJMXTest"

type JMXTomcatJVMTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*JMXTomcatJVMTestRunner)(nil)

func (t *JMXTomcatJVMTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.GetMeasuredMetrics()
	testResults := make([]status.TestResult, len(metricsToFetch))
	for i, metricName := range metricsToFetch {
		testResults[i] = t.validateJMXMetric(metricName)
	}

	return status.TestGroupResult{
		Name:        t.GetTestName(),
		TestResults: testResults,
	}
}

func (t *JMXTomcatJVMTestRunner) GetTestName() string {
	return "JMXTomcatJVM"
}

func (t *JMXTomcatJVMTestRunner) GetAgentConfigFileName() string {
	return "jmx_tomcat_jvm_config.json"
}

func (t *JMXTomcatJVMTestRunner) GetAgentRunDuration() time.Duration {
	return 5 * time.Minute
}

func (t *JMXTomcatJVMTestRunner) SetupBeforeAgentRun() error {
	err := t.BaseTestRunner.SetupBeforeAgentRun()
	if err != nil {
		return err
	}

	log.Println("Checking Java version:")
	javaVersion, _ := common.RunCommand("java -version 2>&1")
	log.Printf("Java version:\n%s", javaVersion)

	log.Println("Setting up JVM and Tomcat with JMX enabled...")
	startJMXCommands := []string{
		"nohup java -Dcom.sun.management.jmxremote -Dcom.sun.management.jmxremote.port=2030 " +
			"-Dcom.sun.management.jmxremote.local.only=false -Dcom.sun.management.jmxremote.authenticate=false " +
			"-Dcom.sun.management.jmxremote.ssl=false -Dcom.sun.management.jmxremote.rmi.port=2030 " +
			"-Dcom.sun.management.jmxremote.host=0.0.0.0 -Djava.rmi.server.hostname=0.0.0.0 " +
			"-Dserver.port=8090 -Dspring.application.admin.enabled=true " +
			"-Dserver.tomcat.mbeanregistry.enabled=true -Dmanagement.endpoints.jmx.exposure.include=* " +
			"-XX:+UseConcMarkSweepGC -verbose:gc " +
			"-jar jars/spring-boot-web-starter-tomcat.jar > /tmp/spring-boot-web-starter-tomcat-jar.txt 2>&1 &",
	}

	err = common.RunCommands(startJMXCommands)
	if err != nil {
		return err
	}

	log.Println("Waiting 20 seconds for Tomcat/JMX to initialize...")
	time.Sleep(20 * time.Second)
	return nil
}

func (t *JMXTomcatJVMTestRunner) GetMeasuredMetrics() []string {
	return []string{
		"jvm.threads.count",
		"jvm.memory.pool.used",
		"jvm.memory.pool.max",
		"jvm.memory.pool.init",
		"jvm.memory.pool.committed",
		"jvm.memory.nonheap.used",
		"jvm.memory.nonheap.max",
		"jvm.memory.nonheap.init",
		"jvm.memory.nonheap.committed",
		"jvm.memory.heap.used",
		"jvm.memory.heap.max",
		"jvm.memory.heap.init",
		"jvm.memory.heap.committed",
		"jvm.gc.collections.elapsed",
		"jvm.gc.collections.count",
		"jvm.classes.loaded",
		"tomcat.traffic",
		"tomcat.threads",
		"tomcat.sessions",
		"tomcat.request_count",
		"tomcat.processing_time",
		"tomcat.max_time",
		"tomcat.errors",
	}
}

func (t *JMXTomcatJVMTestRunner) validateJMXMetric(metricName string) status.TestResult {
	testResult := status.TestResult{
		Name:   metricName,
		Status: status.FAILED,
	}

	dims, failed := t.DimensionFactory.GetDimensions([]dimension.Instruction{
		{
			Key:   "InstanceId",
			Value: dimension.UnknownDimensionValue(),
		},
	})

	if len(failed) > 0 {
		return testResult
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(jmxNamespace, metricName, dims, metric.AVERAGE, metric.HighResolutionStatPeriod)
	log.Printf("Fetched values for %s: %v", metricName, values)
	if err != nil {
		log.Printf("Failed to fetch metric %s: %v", metricName, err)
		return testResult
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, -1) {
		log.Printf("Metric %s did not meet expected value threshold", metricName)
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
