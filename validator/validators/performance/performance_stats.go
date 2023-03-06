// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package performance

import (
	"log"
	"math"
	"sort"
)

type Stats struct {
	Average float64
	P99     float64 //99% percent process
	Max     float64
	Min     float64
	Period  int //in seconds
	Std     float64
}

/*
CalculateMetricStatisticsBasedOnDataAndPeriod takes in an array of data and returns the average, min, max, p99, and stdev of the data.
statistics are calculated this way instead of using GetMetricStatistics API because GetMetricStatistics would require multiple
API calls as only one metric can be requested/processed at a time whereas all metrics can be requested in one GetMetricData request.
*/
func CalculateMetricStatisticsBasedOnDataAndPeriod(data []float64, dataPeriod float64) Stats {
	length := len(data)
	if length == 0 {
		return Stats{}
	}

	sort.Float64s(data)

	min := data[0]
	max := data[length-1]

	sum := 0.0
	for _, value := range data {
		sum += value
	}

	avg := sum / float64(length)

	if length < 99 {
		log.Println("Note: less than 99 values given, p99 value will be equal the max value")
	}
	p99Index := int(float64(length)*.99) - 1
	p99Val := data[p99Index]

	stdDevSum := 0.0
	for _, value := range data {
		stdDevSum += math.Pow(avg-value, 2)
	}

	return Stats{
		Average: avg,
		Max:     max,
		Min:     min,
		P99:     p99Val,
		Std:     math.Sqrt(stdDevSum / float64(length)),
		Period:  int(dataPeriod / float64(length)),
	}
}
