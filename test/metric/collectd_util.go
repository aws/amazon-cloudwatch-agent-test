// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric

import (
	"encoding/json"
	"log"
	"strings"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

type Entity struct {
	Type          string        `json:"__type"`
	Attributes    Attributes    `json:"Attributes"`
	KeyAttributes KeyAttributes `json:"KeyAttributes"`
}

type Attributes struct {
	ServiceNameSource string `json:"AWS.ServiceNameSource"`
}

type KeyAttributes struct {
	Environment string `json:"Environment"`
	Type        string `json:"Type"`
	Name        string `json:"Name"`
}

type Dimension struct {
	Name  string
	Value string
}

func BuildCollectDRequestBody(namespace, metricName string) ([]byte, error) {
	metricType := GetCollectDMetricType(metricName)
	instanceId := awsservice.GetInstanceId()

	dimensions := []Dimension{
		{
			Name:  "InstanceId",
			Value: instanceId,
		},
		{
			Name:  "type",
			Value: metricType,
		},
	}

	request := struct {
		Namespace  string      `json:"Namespace"`
		MetricName string      `json:"MetricName"`
		Dimensions []Dimension `json:"Dimensions"`
	}{
		Namespace:  namespace,
		MetricName: metricName,
		Dimensions: dimensions,
	}

	jsonBytes, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	return jsonBytes, nil
}

func GetCollectDMetricType(metricName string) string {
	split := strings.Split(metricName, "_")
	if len(split) != 4 {
		log.Printf("unexpected metric name format, %s", metricName)
	}
	metricType := split[1]
	return metricType
}
