// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package metric

import "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

type MetricValues []float64

const (
	AVERAGE                  types.Statistic = "Average"
	SAMPLE_COUNT             types.Statistic = "SampleCount"
	MINIMUM                  types.Statistic = "Minimum"
	MAXIMUM                  types.Statistic = "Maximum"
	SUM                      types.Statistic = "Sum"
	HighResolutionStatPeriod                 = 10
	MinuteStatPeriod                         = 60
)
