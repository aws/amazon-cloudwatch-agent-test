package common

import (
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
	"time"
)

type MetricGenerator struct {
	counterValues []int64
	gaugeValues   []float64
	summaryValues []float64
	config        PrometheusConfig
}

func NewMetricGenerator(config PrometheusConfig) *MetricGenerator {
	return &MetricGenerator{
		counterValues: make([]int64, config.CounterCount),
		gaugeValues:   make([]float64, config.GaugeCount),
		summaryValues: make([]float64, config.SummaryCount),
		config:        config,
	}
}

func (mg *MetricGenerator) generateMetrics(w http.ResponseWriter) {

	// Generate counter metrics
	for i := 0; i < mg.config.CounterCount; i++ {
		fmt.Fprintf(w, "# HELP prometheus_test_counter A test counter metric\n")
		fmt.Fprintf(w, "# TYPE prometheus_test_counter counter\n")
		fmt.Fprintf(w, "prometheus_test_counter{instance_id=\"%s\"} %d\n",
			mg.config.InstanceID, atomic.LoadInt64(&mg.counterValues[i]))
	}

	// Generate gauge metrics
	for i := 0; i < mg.config.GaugeCount; i++ {
		fmt.Fprintf(w, "# HELP prometheus_test_gauge A test gauge metric\n")
		fmt.Fprintf(w, "# TYPE prometheus_test_gauge gauge\n")
		fmt.Fprintf(w, "prometheus_test_gauge{instance_id=\"%s\"} %f\n",
			mg.config.InstanceID, mg.gaugeValues[i])
	}

	// Generate summary metrics
	for i := 0; i < mg.config.SummaryCount; i++ {
		fmt.Fprintf(w, "# HELP prometheus_test_summary A test summary metric\n")
		fmt.Fprintf(w, "# TYPE prometheus_test_summary summary\n")
		fmt.Fprintf(w, "prometheus_test_summary_sum{instance_id=\"%s\"} %f\n",
			mg.config.InstanceID, mg.summaryValues[i])
	}

	// Log metric generation for debugging
	log.Printf("Generated %d counter metrics, %d gauge metrics, %d summary metrics",
		mg.config.CounterCount, mg.config.GaugeCount, mg.config.SummaryCount)

}

func (mg *MetricGenerator) updateMetrics() {
	ticker := time.NewTicker(mg.config.UpdateInterval)
	defer ticker.Stop()

	for range ticker.C {
		// Update counters (always increasing)
		for i := range mg.counterValues {
			atomic.AddInt64(&mg.counterValues[i], 1)
		}

		// Update gauges (fluctuating values)
		for i := range mg.gaugeValues {
			mg.gaugeValues[i] = float64(50 + time.Now().Unix()%10)
		}

		// Update summaries (increasing values)
		for i := range mg.summaryValues {
			mg.summaryValues[i] += 1.0
		}

		// Log updates for debugging
		log.Printf("Updated metrics at %v", time.Now())
	}
}

// Helper function to start the metric generator server
func StartMetricGenerator(config PrometheusConfig) error {
	mg := NewMetricGenerator(config)

	// Start updating metrics in background
	go mg.updateMetrics()

	// Set up HTTP handler
	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		mg.generateMetrics(w)
	})

	// Start server
	addr := fmt.Sprintf(":%d", config.Port)
	log.Printf("Starting metric server on %s", addr)
	return http.ListenAndServe(addr, nil)
}

// Helper function to validate metric generation
func ValidateMetricGeneration(port int) error {
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/metrics", port))
	if err != nil {
		return fmt.Errorf("failed to get metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}
