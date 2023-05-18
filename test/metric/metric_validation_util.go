package metric

import "log"

// IsAllValuesGreaterThanOrEqualToExpectedValue will compare if the given array is larger than 0
// and check if the average value for the array is not la
// https://github.com/aws/amazon-cloudwatch-agent-test/pull/162
func IsAllValuesGreaterThanOrEqualToExpectedValue(metricName string, values []float64, expectedValue float64) bool {
	if len(values) == 0 {
		log.Printf("No values found %v", metricName)
		return false
	}

	totalSum := 0.0
	for _, value := range values {
		if value < 0 {
			log.Printf("Values are not all greater than or equal to zero for %s", metricName)
			return false
		}
		totalSum += value
	}
	metricErrorBound := 0.1
	metricAverageValue := totalSum / float64(len(values))
	upperBoundValue := expectedValue * (1 + metricErrorBound)
	lowerBoundValue := expectedValue * (1 - metricErrorBound)
	if expectedValue > 0 && (metricAverageValue > upperBoundValue || metricAverageValue < lowerBoundValue) {
		log.Printf("The average value %f for metric %s are not within bound [%f, %f]",
			metricAverageValue, metricName, lowerBoundValue, upperBoundValue)
		return false
	}

	log.Printf("The average value %f for metric %s are within bound [%f, %f]",
		metricAverageValue, metricName, lowerBoundValue, upperBoundValue)
	return true
}
