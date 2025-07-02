package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
)

var (
	counter uint64
	port    int
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

	// Static values for histogram
	staticSum := 55.0    // Sum for one scrape
	staticCount := 1     // Sample count for one scrape

	// Output sum and count
	fmt.Fprintf(w, "prometheus_test_histogram_sum{include=\"yes\",prom_type=\"histogram\"} %f\n", staticSum)
	fmt.Fprintf(w, "prometheus_test_histogram_count{include=\"yes\",prom_type=\"histogram\"} %d\n", staticCount)

	// Output histogram buckets with static counts
	fmt.Fprintf(w, "prometheus_test_histogram_bucket{include=\"yes\",le=\"1.0\",prom_type=\"histogram\"} %d\n", 1)
	fmt.Fprintf(w, "prometheus_test_histogram_bucket{include=\"yes\",le=\"2.5\",prom_type=\"histogram\"} %d\n", 1)
	fmt.Fprintf(w, "prometheus_test_histogram_bucket{include=\"yes\",le=\"5.0\",prom_type=\"histogram\"} %d\n", 1)
	fmt.Fprintf(w, "prometheus_test_histogram_bucket{include=\"yes\",le=\"7.5\",prom_type=\"histogram\"} %d\n", 1)
	fmt.Fprintf(w, "prometheus_test_histogram_bucket{include=\"yes\",le=\"10.0\",prom_type=\"histogram\"} %d\n", 1)
	fmt.Fprintf(w, "prometheus_test_histogram_bucket{include=\"yes\",le=\"+Inf\",prom_type=\"histogram\"} %d\n", 1)
}

func main() {
	flag.Parse()
	log.Printf("Starting Prometheus metrics server on :%d\n", port)
	http.HandleFunc("/metrics", metricsHandler)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
		log.Fatal(err)
	}
}
