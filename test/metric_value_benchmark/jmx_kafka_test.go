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

type JMXKafkaTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*JMXKafkaTestRunner)(nil)

func (t *JMXKafkaTestRunner) Validate() status.TestGroupResult {
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

func (t *JMXKafkaTestRunner) GetTestName() string {
	return "JMXKafka"
}

func (t *JMXKafkaTestRunner) GetAgentConfigFileName() string {
	return "jmx_kafka_config.json"
}

func (t *JMXKafkaTestRunner) GetAgentRunDuration() time.Duration {
	return 5 * time.Minute
}

func (t *JMXKafkaTestRunner) SetupBeforeAgentRun() error {
	err := t.BaseTestRunner.SetupBeforeAgentRun()
	if err != nil {
		return err
	}

	log.Println("set up zookeeper and kafka")
	startJMXCommands := []string{
		"export KAFKA_VERSION=curl https://dlcdn.apache.org/kafka/ | grep -oE '\\d\\.\\d\\.\\d' | tail -1",
		"curl https://dlcdn.apache.org/kafka/$KAFKA_VERSION/kafka_2.13-$KAFKA_VERSION.tgz -o kafka_2.13-latest.tgz",
		"tar -xzf kafka_2.13-latest.tgz",
		"echo 'export JMX_PORT=2000'|cat - kafka_2.13-latest/bin/kafka-server-start.sh > /tmp/kafka-server-start.sh && mv /tmp/kafka-server-start.sh kafka_2.13-latest/bin/kafka-server-start.sh",
		"echo 'export JMX_PORT=2010'|cat - kafka_2.13-latest/bin/kafka-console-consumer.sh > /tmp/kafka-console-consumer.sh && mv /tmp/kafka-console-consumer.sh kafka_2.13-latest/bin/kafka-console-consumer.sh",
		"echo 'export JMX_PORT=2020'|cat - kafka_2.13-latest/bin/kafka-console-producer.sh > /tmp/kafka-console-producer.sh && mv /tmp/kafka-console-producer.sh kafka_2.13-latest/bin/kafka-console-producer.sh",
		"sudo chmod +x kafka_2.13-latest/bin/kafka-run-class.sh",
		"sudo chmod +x kafka_2.13-latest/bin/kafka-server-start.sh",
		"sudo chmod +x kafka_2.13-latest/bin/kafka-console-consumer.sh",
		"sudo chmod +x kafka_2.13-latest/bin/kafka-console-producer.sh",
		"(yes | nohup kafka_2.13-latest/bin/kafka-console-producer.sh --topic quickstart-events --bootstrap-server localhost:9092) > /tmp/kafka-console-producer-logs.txt 2>&1 &",
		"kafka_2.13-latest/bin/kafka-console-consumer.sh --topic quickstart-events --from-beginning --bootstrap-server localhost:9092 > /tmp/kafka-console-consumer-logs.txt 2>&1 &",
		"curl https://dlcdn.apache.org/zookeeper/zookeeper-3.8.4/apache-zookeeper-3.8.4-bin.tar.gz -o apache-zookeeper-3.8.4-bin.tar.gz",
		"tar -xzf apache-zookeeper-3.8.4-bin.tar.gz",
		"mkdir apache-zookeeper-3.8.4-bin/data",
		"touch apache-zookeeper-3.8.4-bin/conf/zoo.cfg",
		"echo -e 'tickTime = 2000\ndataDir = ../data\nclientPort = 2181\ninitLimit = 5\nsyncLimit = 2\n' >> apache-zookeeper-3.8.4-bin/conf/zoo.cfg",
		"sudo apache-zookeeper-3.8.4-bin/bin/zkServer.sh start",
		"sudo kafka_2.13-latest/bin/kafka-server-start.sh kafka_2.13-latest/config/server.properties > /tmp/kafka-server-start-logs.txt 2>&1 &",
	}

	err = common.RunCommands(startJMXCommands)
	if err != nil {
		return err
	}
	return nil
}

func (t *JMXKafkaTestRunner) GetMeasuredMetrics() []string {
	return []string{
		"kafka.unclean.election.rate",
		"kafka.request.time.total",
		"kafka.request.time.avg",
		"kafka.request.time.99p",
		"kafka.request.time.50p",
		"kafka.request.queue",
		"kafka.request.failed",
		"kafka.request.count",
		"kafka.purgatory.size",
		"kafka.partition.under_replicated",
		"kafka.partition.offline",
		"kafka.partition.count",
		"kafka.network.io",
		"kafka.message.count",
		"kafka.max.lag",
		"kafka.leader.election.rate",
		"kafka.isr.operation.count",
		"kafka.controller.active.count",
		"kafka.consumer.total.records-consumed-rate",
		"kafka.consumer.total.bytes-consumed-rate",
		"kafka.consumer.records-consumed-rate",
		"kafka.consumer.fetch-rate",
		"kafka.consumer.bytes-consumed-rate",
		"kafka.producer.io-wait-time-ns-avg",
		"kafka.producer.record-retry-rate",
		"kafka.producer.compression-rate",
		"kafka.producer.outgoing-byte-rate",
		"kafka.producer.request-rate",
		"kafka.producer.byte-rate",
		"kafka.producer.request-latency-avg",
		"kafka.producer.response-rate",
		"kafka.producer.record-error-rate",
		"kafka.producer.record-send-rate",
	}
}

func (t *JMXKafkaTestRunner) validateJMXMetric(metricName string) status.TestResult {
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

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(metricName, values, 0) {
		return testResult
	}

	testResult.Status = status.SUCCESSFUL
	return testResult
}
