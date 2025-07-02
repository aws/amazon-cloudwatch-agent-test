package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"sort"
	"sync/atomic"
)

var (
	counter uint64
	port    int

	// Fixed values for histogram
	histogramValues = []float64{
		1.0, 2.0, 3.0, 4.0, 5.0,
		6.0, 7.0, 8.0, 9.0, 10.0,
	}
)

func init() {
	flag.IntVar(&port, "port", 8101, "Port to listen on")
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	// Get current counter value and increment
	currentCounter := atomic.LoadUint64(&counter)
	atomic.AddUint64(&counter, 1)

	// Output untyped metric
	fmt.Fprintf(w, "prometheus_test_untyped{include=\"yes\",prom_type=\"untyped\"} %d\n",
		currentCounter%100+1)

	// Output counter metric
	fmt.Fprintf(w, "# TYPE prometheus_test_counter counter\n")
	fmt.Fprintf(w, "prometheus_test_counter{include=\"yes\",prom_type=\"counter\"} %d\n",
		currentCounter*5) // Make it increase by 5 each time

	// Output gauge metric
	fmt.Fprintf(w, "# TYPE prometheus_test_gauge gauge\n")
	fmt.Fprintf(w, "prometheus_test_gauge{include=\"yes\",prom_type=\"gauge\"} %f\n",
		float64(currentCounter%1000))

	// Output histogram metrics
	fmt.Fprintf(w, "# TYPE prometheus_test_histogram histogram\n")

	// Calculate sum and count
	sum := 0.0
	for _, v := range histogramValues {
		sum += v
	}

	fmt.Fprintf(w, "prometheus_test_histogram_sum{include=\"yes\",prom_type=\"histogram\"} %f\n", sum)
	fmt.Fprintf(w, "prometheus_test_histogram_count{include=\"yes\",prom_type=\"histogram\"} %d\n",
		len(histogramValues))

	// Output histogram buckets
	count := 0
	buckets := []float64{1.0, 2.5, 5.0, 7.5, 10.0}
	for _, bound := range buckets {
		for _, v := range histogramValues {
			if v <= bound {
				count++
			}
		}
		fmt.Fprintf(w, "prometheus_test_histogram_bucket{include=\"yes\",le=\"%g\",prom_type=\"histogram\"} %d\n",
			bound, count)
		count = 0
	}
	fmt.Fprintf(w, "prometheus_test_histogram_bucket{include=\"yes\",le=\"+Inf\",prom_type=\"histogram\"} %d\n",
		len(histogramValues))

	// Output quantiles
	sorted := make([]float64, len(histogramValues))
	copy(sorted, histogramValues)
	sort.Float64s(sorted)

	fmt.Fprintf(w, "prometheus_test_histogram_quantile{quantile=\"0.50\",include=\"yes\",prom_type=\"histogram\"} %f\n",
		sorted[len(sorted)/2])
	fmt.Fprintf(w, "prometheus_test_histogram_quantile{quantile=\"0.90\",include=\"yes\",prom_type=\"histogram\"} %f\n",
		sorted[int(float64(len(sorted))*0.9)])
	fmt.Fprintf(w, "prometheus_test_histogram_quantile{quantile=\"0.95\",include=\"yes\",prom_type=\"histogram\"} %f\n",
		sorted[int(float64(len(sorted))*0.95)])
}

func main() {
	flag.Parse()
	log.Printf("Starting Prometheus metrics server on :%d\n", port)
	http.HandleFunc("/metrics", metricsHandler)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
		log.Fatal(err)
	}
}
