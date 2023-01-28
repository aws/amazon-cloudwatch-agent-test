// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package models // import "github.com/aws/amazon-cloudwatch-agent-test/validator/models"

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type ValidateConfig interface {
	GetOtelConfig() ([]string, []string, []string)
	GetValidateType() string
	GetTestCase() string
	GetDataType() string
	GetDataRate() int
	GetCloudWatchAgentConfigPath() string
	GetDataPointPeriod() time.Duration
}
type validatorConfig struct {
	receivers  []string `yaml:"receivers"`
	processors []string `yaml:"processors"`
	exporters  []string `yaml:"exporters"`

	testCase        string `yaml:"test_case"`
	validateType    string `yaml:"validate_type"`
	dataType        string `yaml:"data_type"`
	dataRate        int    `yaml:"data_rate"`
	datapointPeriod int    `yaml:"datapoint_period"`

	cwaConfigPath string `yaml:"cloudwatch_agent_config"`
}

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

	return &vConfig, nil
}

func (v *validatorConfig) GetTestCase() string {
	return v.testCase
}

func (v *validatorConfig) GetValidateType() string {
	return v.validateType
}

func (v *validatorConfig) GetOtelConfig() ([]string, []string, []string) {
	return v.receivers, v.processors, v.exporters
}

func (v *validatorConfig) GetDataType() string {
	return v.dataType
}

func (v *validatorConfig) GetDataRate() int {
	return v.dataRate
}

func (v *validatorConfig) GetCloudWatchAgentConfigPath() string {
	return v.cwaConfigPath
}

func (v *validatorConfig) GetDataPointPeriod() time.Duration {
	return time.Duration(v.datapointPeriod) * time.Minute
}
