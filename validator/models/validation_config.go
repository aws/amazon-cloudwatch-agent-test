// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package models // import "github.com/aws/amazon-cloudwatch-agent-test/validator/models"

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type ValidateConfig interface {
	GetPluginsConfig() string
	GetValidateType() string
	GetTestCase() string
	GetDataType() string
	GetNumberMonitoredLogs() int
	GetDataRate() int
	GetCloudWatchAgentConfigPath() string
	GetAgentCollectionPeriod() time.Duration
	GetMetricNamespace() string
	GetMetricValidation() []MetricValidation
	GetLogValidation() []LogValidation
	GetCommitInformation() (string, int64)
}
type validatorConfig struct {
	Receiver string `yaml:"receivers"` // Receivers that agent needs to tests

	TestCase string `yaml:"test_case"` // Test case name

	// Validate type for the test https://github.com/aws/amazon-cloudwatch-agent-test/blob/39a9e16c70f07a17c43c0630647158cd496bd168/validator/validators/validator.go#L15-L24
	ValidateType          string `yaml:"validate_type"`
	DataType              string `yaml:"data_type"`               // Only supports metrics/logs/traces
	NumberMonitoredLogs   int    `yaml:"number_monitored_logs"`   // Number of logs to be monitored
	ValuesPerMinute       string `yaml:"values_per_minute"`       // Number of metrics to be sent or number of log lines to write
	AgentCollectionPeriod int    `yaml:"agent_collection_period"` // Number of seconds the agent should run and collect the metrics

	ConfigPath string `yaml:"cloudwatch_agent_config"`

	MetricNamespace  string             `yaml:"metric_namespace"`
	MetricValidation []MetricValidation `yaml:"metric_validation"`
	LogValidation    []LogValidation    `yaml:"log_validation"`

	CommitHash string `yaml:"commit_hash"`
	CommitDate string `yaml:"commit_date"`
}

type LogValidation struct {
	LogValue  string `mapstructure:"log_value,omitempty"`
	LogLines  int    `mapstructure:"log_lines,omitempty"`
	LogStream string `mapstructure:"log_stream,omitempty"`
}

type MetricValidation struct {
	MetricName      string            `mapstructure:"metric_name,omitempty"`
	MetricDimension []MetricDimension `mapstructure:"metric_dimension,omitempty"`
	MetricValue     float64           `mapstructure:"metric_value,omitempty"`
}

type MetricDimension struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

var _ ValidateConfig = (*validatorConfig)(nil)

func NewValidateConfig(configPath string) (*validatorConfig, error) {
	configPathBytes, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("%v with file %s", err, configPath)
	}

	vConfig := validatorConfig{}
	err = yaml.Unmarshal(configPathBytes, &vConfig)
	if err != nil {
		return nil, err
	}
	log.Printf("Parameters validation for %v", vConfig)
	return &vConfig, nil
}

// GetTestCase return the test case name
func (v *validatorConfig) GetTestCase() string {
	return v.TestCase
}

// GetTestCase return the validation type (e.g stress https://github.com/aws/amazon-cloudwatch-agent-test/pull/109/files#diff-36fa5ec31f40a4d9a878623ba1993272853ab2125e64152317da2a66cc7365d6R17-R18)
func (v *validatorConfig) GetValidateType() string {
	return v.ValidateType
}

// GetPluginsConfig returns the agent plugin being used or need to validate (e.g statsd, collectd, cpu)
func (v *validatorConfig) GetPluginsConfig() string {
	return v.Receiver
}

// GetPluginsConfig returns the type needs to validate or send. Only supports metrics, traces, logs
func (v *validatorConfig) GetDataType() string {
	return v.DataType
}

// GetDataRate returns number of metrics to be sent or number of log lines to write
func (v *validatorConfig) GetDataRate() int {
	if dataRate, err := strconv.ParseInt(v.ValuesPerMinute, 10, 64); err == nil {
		return int(dataRate)
	}
	return 0
}

// GetNumberMonitoredLogs returns number of log to be monitored by cloudwatchagent so the validator configuration will setup the agent config dynamically
func (v *validatorConfig) GetNumberMonitoredLogs() int {
	return v.NumberMonitoredLogs
}

// GetNumberMonitoredLogs returns the cloudwatch agent path configuration
func (v *validatorConfig) GetCloudWatchAgentConfigPath() string {
	return v.ConfigPath
}

// GetNumberMonitoredLogs returns the number of seconds the agent should run and collect the metrics
func (v *validatorConfig) GetAgentCollectionPeriod() time.Duration {
	return time.Duration(v.AgentCollectionPeriod) * time.Second
}

// GetNumberMonitoredLogs returns the namespace that metrics need to be validated
func (v *validatorConfig) GetMetricNamespace() string {
	return v.MetricNamespace
}

// GetMetricValidation returns the metrics need for validation
func (v *validatorConfig) GetMetricValidation() []MetricValidation {
	return v.MetricValidation
}

// GetLogValidation returns the logs need for validation
func (v *validatorConfig) GetLogValidation() []LogValidation {
	return v.LogValidation
}

func (v *validatorConfig) GetCommitInformation() (string, int64) {
	commitDate, _ := strconv.ParseInt(v.CommitDate, 10, 64)
	return v.CommitHash, commitDate
}
