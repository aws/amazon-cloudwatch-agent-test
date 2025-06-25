package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync/atomic"
	"time"
)

var (
	counter   uint64
	histogram []float64
	port      int
)

func init() {
	flag.IntVar(&port, "port", 8101, "Port to listen on")
}

func updateMetrics() {
	for {
		atomic.AddUint64(&counter, 1)
		value := rand.Float64() * 10
		histogram = append(histogram, value)
		time.Sleep(1 * time.Second)
	}
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	currentCounter := atomic.LoadUint64(&counter)

	// Calculate histogram stats
	var sum float64
	buckets := make(map[float64]int)
	bounds := []float64{0, 0.5, 2.5, 5.0}

	for _, v := range histogram {
		sum += v
		for _, bound := range bounds {
			if v <= bound {
				buckets[bound]++
			}
		}
	}

	fmt.Fprintf(w, "# TYPE prometheus_test_counter counter\n")
	fmt.Fprintf(w, "prometheus_test_counter{include=\"yes\",prom_type=\"counter\"} %d\n", currentCounter)

	fmt.Fprintf(w, "# TYPE prometheus_test_gauge gauge\n")
	fmt.Fprintf(w, "prometheus_test_gauge{include=\"yes\",prom_type=\"gauge\"} %f\n", rand.Float64()*1000)

	fmt.Fprintf(w, "# TYPE prometheus_test_histogram histogram\n")
	fmt.Fprintf(w, "prometheus_test_histogram_sum{include=\"yes\",prom_type=\"histogram\"} %f\n", sum)
	fmt.Fprintf(w, "prometheus_test_histogram_count{include=\"yes\",prom_type=\"histogram\"} %d\n", len(histogram))

	count := 0
	for _, bound := range bounds {
		count += buckets[bound]
		fmt.Fprintf(w, "prometheus_test_histogram_bucket{include=\"yes\",le=\"%g\",prom_type=\"histogram\"} %d\n", bound, count)
	}
	fmt.Fprintf(w, "prometheus_test_histogram_bucket{include=\"yes\",le=\"+Inf\",prom_type=\"histogram\"} %d\n", len(histogram))
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
