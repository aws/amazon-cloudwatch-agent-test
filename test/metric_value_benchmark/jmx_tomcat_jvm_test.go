// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_value_benchmark

import (
	"log"
	"sort"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	jmxNamespace = "MetricValueBenchmarkJMXTest"

	mb = 1024 * 1024
	gb = 1024 * mb
)

type JMXTomcatJVMTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*JMXTomcatJVMTestRunner)(nil)

func (t *JMXTomcatJVMTestRunner) Validate() status.TestGroupResult {
	metricsToFetch := t.getExpectedMetricBounds()
	testResults := make([]status.TestResult, 0, len(metricsToFetch))
	for metricName, bounds := range metricsToFetch {
		testResults = append(testResults, t.validateJMXMetric(metricName, bounds))
	}

	sort.Slice(testResults, func(i, j int) bool {
		return testResults[i].Name < testResults[j].Name
	})

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
	return 2 * time.Minute
}

func (t *JMXTomcatJVMTestRunner) SetupBeforeAgentRun() error {
	err := t.BaseTestRunner.SetupBeforeAgentRun()
	if err != nil {
		return err
	}

	log.Println("set up jvm and tomcat")
	startJMXCommands := []string{
		"nohup java -Dcom.sun.management.jmxremote -Dcom.sun.management.jmxremote.port=2030 -Dcom.sun.management.jmxremote.local.only=false -Dcom.sun.management.jmxremote.authenticate=false -Dcom.sun.management.jmxremote.ssl=false -Dcom.sun.management.jmxremote.rmi.port=2030  -Dcom.sun.management.jmxremote.host=0.0.0.0  -Djava.rmi.server.hostname=0.0.0.0 -Dserver.port=8090 -Dspring.application.admin.enabled=true -jar jars/spring-boot-web-starter-tomcat.jar > /tmp/spring-boot-web-starter-tomcat-jar.txt 2>&1 &",
	}

	err = common.RunCommands(startJMXCommands)
	if err != nil {
		return err
	}
	return nil
}

func (t *JMXTomcatJVMTestRunner) getExpectedMetricBounds() map[string][2]float64 {
	return map[string][2]float64{
		"jvm.classes.loaded": buildBounds(7760, 100),

		"jvm.gc.collections.elapsed": {},
		"jvm.gc.collections.count":   {},

		"jvm.memory.heap.committed": buildBounds(68.2*mb, 1*mb),
		"jvm.memory.heap.init":      buildBounds(63*mb, 10*mb),
		"jvm.memory.heap.max":       buildBounds(1.1*gb, 100*mb),
		"jvm.memory.heap.used":      buildBounds(43.4*mb, 0),

		"jvm.memory.nonheap.committed": buildBounds(68*mb, 1*mb),
		"jvm.memory.nonheap.init":      buildBounds(7.5*mb, 0.5*mb),
		"jvm.memory.nonheap.max":       {-1, -1},
		"jvm.memory.nonheap.used":      buildBounds(64*mb, 5*mb),

		"jvm.memory.pool.committed": buildBounds(20*mb, 5*mb),
		"jvm.memory.pool.init":      buildBounds(9*mb, 2*mb),
		"jvm.memory.pool.max":       buildBounds(295*mb, 50*mb),
		"jvm.memory.pool.used":      buildBounds(15*mb, 2*mb),

		"jvm.threads.count": buildBounds(25, 5),

		"tomcat.errors":          {},
		"tomcat.max_time":        {},
		"tomcat.processing_time": {},
		"tomcat.request_count":   {},
		"tomcat.sessions":        {},
		"tomcat.threads":         {},
		"tomcat.traffic":         {},
	}
}

func (t *JMXTomcatJVMTestRunner) validateJMXMetric(metricName string, bounds [2]float64) status.TestResult {
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
	log.Printf("metric values are %v", values)
	if err != nil {
		log.Printf("err: %v\n", err)
		return testResult
	}

	if err = metric.IsAverageWithinBounds(values, bounds); err != nil {
		log.Printf("err: %v\n", err)
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}

func buildBounds(base, variance float64) [2]float64 {
	return [2]float64{base - variance, base + variance}
}
