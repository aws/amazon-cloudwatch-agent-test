// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sort"
	"sync/atomic"
	"time"
)

var (
	counter   uint64
	histogram []float64
	untyped   uint64
	port      int

	// Fixed values for predictable testing
	counterIncrement = uint64(5)
	// Expanded histogram values to better test quantiles
	histogramValues = []float64{
		1.0, 1.5, 2.0, 2.5, 3.0, // Lower quantile values
		4.0, 4.5, 5.0, 5.5, 6.0, // Mid-range values
		7.0, 7.5, 8.0, 8.5, 9.0, // Upper quantile values
	}
	histogramBuckets = []float64{1.0, 2.5, 5.0, 7.5, 10.0} // Bucket boundaries
	untypedValue     = uint64(50)
	gaugeValue       = 500.0
)

func init() {
	flag.IntVar(&port, "port", 8101, "Port to listen on")
}

func updateMetrics() {
	for {
		atomic.AddUint64(&counter, 1)
		value := rand.Float64() * 10
		histogram = append(histogram, value)

		atomic.StoreUint64(&untyped, uint64(rand.Intn(100)+1))

		time.Sleep(1 * time.Second)
	}
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	currentCounter := atomic.LoadUint64(&counter)
	currentUntyped := atomic.LoadUint64(&untyped)

	// Calculate histogram stats
	var sum float64
	buckets := make(map[float64]int)

	// Initialize buckets
	for _, bound := range histogramBuckets {
		buckets[bound] = 0
	}

	// Calculate histogram metrics
	for _, v := range histogram {
		sum += v
		for _, bound := range histogramBuckets {
			if v <= bound {
				buckets[bound]++
			}
		}
	}
	fmt.Fprintf(w, "prometheus_test_untyped{include=\"yes\",prom_type=\"untyped\"} %d\n", currentUntyped)

	fmt.Fprintf(w, "# TYPE prometheus_test_counter counter\n")
	fmt.Fprintf(w, "prometheus_test_counter{include=\"yes\",prom_type=\"counter\"} %d\n", currentCounter)

	fmt.Fprintf(w, "# TYPE prometheus_test_gauge gauge\n")
	fmt.Fprintf(w, "prometheus_test_gauge{include=\"yes\",prom_type=\"gauge\"} %f\n", rand.Float64()*1000)

	fmt.Fprintf(w, "# TYPE prometheus_test_histogram histogram\n")
	fmt.Fprintf(w, "prometheus_test_histogram_sum{include=\"yes\",prom_type=\"histogram\"} %f\n", sum)
	fmt.Fprintf(w, "prometheus_test_histogram_count{include=\"yes\",prom_type=\"histogram\"} %d\n", len(histogram))

	// Output detailed bucket information
	count := 0
	for _, bound := range histogramBuckets {
		count += buckets[bound]
		fmt.Fprintf(w, "prometheus_test_histogram_bucket{include=\"yes\",le=\"%g\",prom_type=\"histogram\"} %d\n", bound, count)
	}
	fmt.Fprintf(w, "prometheus_test_histogram_bucket{include=\"yes\",le=\"+Inf\",prom_type=\"histogram\"} %d\n", len(histogram))

	// Add quantile metrics
	quantiles := calculateQuantiles(histogram)
	fmt.Fprintf(w, "prometheus_test_histogram_quantile{quantile=\"0.50\",include=\"yes\",prom_type=\"histogram\"} %f\n", quantiles[0.50])
	fmt.Fprintf(w, "prometheus_test_histogram_quantile{quantile=\"0.90\",include=\"yes\",prom_type=\"histogram\"} %f\n", quantiles[0.90])
	fmt.Fprintf(w, "prometheus_test_histogram_quantile{quantile=\"0.95\",include=\"yes\",prom_type=\"histogram\"} %f\n", quantiles[0.95])
}

// Helper function to calculate quantiles
func calculateQuantiles(values []float64) map[float64]float64 {
	if len(values) == 0 {
		return nil
	}

	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	quantiles := make(map[float64]float64)
	quantiles[0.50] = sorted[int(float64(len(sorted))*0.50)]
	quantiles[0.90] = sorted[int(float64(len(sorted))*0.90)]
	quantiles[0.95] = sorted[int(float64(len(sorted))*0.95)]

	return quantiles
}

func StartServer() error {
	rand.Seed(time.Now().UnixNano())
	go updateMetrics()
	http.HandleFunc("/metrics", metricsHandler)
	return http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}

func main() {
	flag.Parse()
	log.Printf("Starting Prometheus metrics server on :%d\n", port)
	if err := StartServer(); err != nil {
		log.Fatal(err)
	}
}
