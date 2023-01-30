package performancetest

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/google/uuid"
)

const (
	Namespace                  = "CWAgent"
	DimensionName              = "InstanceId"
	Stat                       = "Average"
	Period                     = 10
	METRIC_PERIOD              = 5 * 60 // this const is in seconds , 5 mins
	PARTITION_KEY              = "Year"
	HASH                       = "Hash"
	COMMIT_DATE                = "CommitDate"
	SHA_ENV                    = "SHA"
	RELEASE_NAME_ENV           = "RELEASE_NAME"
	SHA_DATE_ENV               = "SHA_DATE"
	IS_RELEASE                 = "isRelease"
	TEST_ID                    = "TestID"
	TPS                        = "TPS"
	PERFORMANCE_NUMBER_OF_LOGS = "PERFORMANCE_NUMBER_OF_LOGS"
	RESULTS                    = "Results"
	/*
		TEST_ID is used for version control, in order to make sure the
		item has not changed between item being editted and updated.
		TEST_ID is checked atomicaly.
		 TEST_ID uses UIUD to give unique id to each packet.
	*/
)

// struct that holds statistics on the returned data
type Stats struct {
	Average float64
	P99     float64 //99% percent process
	Max     float64
	Min     float64
	Period  int //in seconds
	Std     float64
	Data    []float64
}

func GetPerformanceMetrics(instanceId string, agentRuntime, logNum, tps int, agentContext context.Context, configPath string) (map[string]interface{}, error) {

	log.Println("Data successfully received from CloudWatch API")

	//craft packet to be sent to database
	packet := make(map[string]interface{})
	//add information about current release/commit
	packet[PARTITION_KEY] = time.Now().Year()
	packet[HASH] = os.Getenv(SHA_ENV) //fmt.Sprintf("%d", time.Now().UnixNano())
	packet[COMMIT_DATE], _ = strconv.Atoi(os.Getenv(SHA_DATE_ENV))
	packet[IS_RELEASE] = false
	//add test metadata
	packet[TEST_ID] = uuid.New().String()
	testSettings := fmt.Sprintf("%d-%d", logNum, tps)
	testMetricResults := make(map[string]Stats)

	//add actual test data with statistics
	for _, result := range metrics.MetricDataResults {
		//convert memory bytes to MB
		if *result.Label == "procstat_memory_rss" {
			for i, val := range result.Values {
				result.Values[i] = val / (1000000)
			}
		}
		stats := CalcStats(result.Values)
		testMetricResults[*result.Label] = stats
	}
	packet[RESULTS] = map[string]map[string]Stats{testSettings: testMetricResults}
	return packet, nil
}

/* CalcStats takes in an array of data and returns the average, min, max, p99, and stdev of the data in a Stats struct
* statistics are calculated this way instead of using GetMetricStatistics API because GetMetricStatistics would require multiple
* API calls as only one metric can be requested/processed at a time whereas all metrics can be requested in one GetMetricData request.
 */
func CalcStats(data []float64) Stats {
	length := len(data)
	if length == 0 {
		return Stats{}
	}

	//make a copy so we aren't modifying original - keeps original data in order of the time
	dataCopy := make([]float64, length)
	copy(dataCopy, data)
	sort.Float64s(dataCopy)

	min := dataCopy[0]
	max := dataCopy[length-1]

	sum := 0.0
	for _, value := range dataCopy {
		sum += value
	}

	avg := sum / float64(length)

	if length < 99 {
		log.Println("Note: less than 99 values given, p99 value will be equal the max value")
	}
	p99Index := int(float64(length)*.99) - 1
	p99Val := dataCopy[p99Index]

	stdDevSum := 0.0
	for _, value := range dataCopy {
		stdDevSum += math.Pow(avg-value, 2)
	}

	stdDev := math.Sqrt(stdDevSum / float64(length))

	statistics := Stats{
		Average: avg,
		Max:     max,
		Min:     min,
		P99:     p99Val,
		Std:     stdDev,
		Period:  int(METRIC_PERIOD / float64(length)),
		Data:    data,
	}

	return statistics
}
