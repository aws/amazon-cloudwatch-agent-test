package main

import (
	"log"
	"net/http"
	"time"
)

func main() {

	generator := NewGenerator()

	monotonicCounter := MetricDefinition{
		Name: "monotonic_counter",
		Type: TypeCounter,
		Help: "A counter that increases forever",
	}

	if err := generator.AddMetric(monotonicCounter); err != nil {
		log.Fatalf("unable to add metric: %v", err)
	}

	sinusoidalGauge := MetricDefinition{
		Name: "sinusoidal_gauge",
		Type: TypeGauge,
		Help: "A gauge that oscillates between -1 and 1",
	}

	if err := generator.AddMetric(sinusoidalGauge); err != nil {
		log.Fatalf("unable to add metric: %v", err)
	}

	poissonHistogram := MetricDefinition{
		Name: "poisson_histogram",
		Type: TypeHistogram,
		Help: "A histogram whose values follow a poisson distribution",
	}

	if err := generator.AddMetric(poissonHistogram); err != nil {
		log.Fatalf("unable to add metric: %v", err)
	}

	// Add a histogram metric
	exponentialSummary := MetricDefinition{
		Name: "exponential_summary",
		Type: TypeSummary,
		Help: "A summary whose values follow an exponential distribution",
	}

	if err := generator.AddMetric(exponentialSummary); err != nil {
		log.Fatalf("unable to add metric: %v", err)
	}

	// Start updating metrics periodically
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for t := range ticker.C {
			if err := generator.UpdateMetrics(t); err != nil {
				log.Printf("Error updating metrics: %v", err)
			}
		}
	}()

	// Start HTTP server
	http.Handle("/metrics", generator)
	log.Printf("Starting server on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
