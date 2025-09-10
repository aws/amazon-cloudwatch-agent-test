package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/protobuf/proto"
)

// MetricType represents supported Prometheus metric types
type MetricType string

const (
	TypeUntyped         MetricType = "untyped"
	TypeCounter         MetricType = "counter"
	TypeGauge           MetricType = "gauge"
	TypeSummary         MetricType = "summary"
	TypeHistogram       MetricType = "histogram"
	TypeNativeHistogram MetricType = "native_histogram"
)

// MetricTimePoint represents a single point in the time series
type MetricTimePoint struct {
	Timestamp time.Time
	Value     float64
	Labels    map[string]string
	// For histograms and summaries
	Buckets map[float64]uint64 `json:",omitempty"`
	Count   uint64             `json:",omitempty"`
	Sum     float64            `json:",omitempty"`
}

// MetricDefinition defines a metric and its time series
type MetricDefinition struct {
	Name       string
	Type       MetricType
	Help       string
	Labels     []string
	TimeSeries []MetricTimePoint
}

// Generator manages the metrics and their generation
type Generator struct {
	metrics  map[string]prometheus.Collector
	registry *prometheus.Registry
	mu       sync.RWMutex
	// Store time series data
	timeSeriesData map[string][]MetricTimePoint
	rand           *rand.Rand
}

var _ http.Handler = (*Generator)(nil)

// NewGenerator creates a new metrics generator
func NewGenerator() *Generator {
	return &Generator{
		metrics:        make(map[string]prometheus.Collector),
		registry:       prometheus.NewRegistry(),
		timeSeriesData: make(map[string][]MetricTimePoint),
		rand:           rand.New(rand.NewSource(0xFEEDBEEF)), // for deterministic results
	}
}

// AddMetric adds a new metric definition to the generator
func (g *Generator) AddMetric(def MetricDefinition) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	var metric prometheus.Collector
	switch def.Type {
	case TypeCounter:
		metric = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: def.Name,
				Help: def.Help,
			},
			def.Labels,
		)

		if err := g.registry.Register(metric); err != nil {
			return fmt.Errorf("failed to register metric: %w", err)
		}

		g.metrics[def.Name] = metric
		g.timeSeriesData[def.Name] = def.TimeSeries

	case TypeGauge:
		metric = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: def.Name,
				Help: def.Help,
			},
			def.Labels,
		)

		if err := g.registry.Register(metric); err != nil {
			return fmt.Errorf("failed to register metric: %w", err)
		}

	case TypeHistogram:
		metric = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: def.Name,
				Help: def.Help,
			},
			def.Labels,
		)

		if err := g.registry.Register(metric); err != nil {
			return fmt.Errorf("failed to register metric: %w", err)
		}

	case TypeSummary:
		metric = prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Name:       def.Name,
				Help:       def.Help,
				Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
			},
			def.Labels,
		)

		if err := g.registry.Register(metric); err != nil {
			return fmt.Errorf("failed to register metric: %w", err)
		}

	default:
		return fmt.Errorf("unsupported metric type: %s", def.Type)
	}

	g.metrics[def.Name] = metric
	return nil
}

// UpdateMetrics updates metric values based on the current timestamp
func (g *Generator) UpdateMetrics(timestamp time.Time) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	for name, metric := range g.metrics {
		switch m := metric.(type) {
		case *prometheus.CounterVec:

			counter, err := m.GetMetricWith(prometheus.Labels{})
			if err != nil {
				return err
			}
			counter.Inc()

		case *prometheus.GaugeVec:
			// Update gauge values
			gauge, err := m.GetMetricWith(prometheus.Labels{})
			if err != nil {
				return err
			}

			// calculate new value using current timestamp. generate a sinusoidal wave with a period of 20 seconds
			newVal := math.Sin(2 * math.Pi * float64(timestamp.Unix()) / 20)
			gauge.Set(newVal)

		case *prometheus.HistogramVec:
			// Update histogram values

			histogram, err := m.GetMetricWith(prometheus.Labels{})
			if err != nil {
				return err
			}

			numObservations := g.rand.Int() % 10

			// Generate a random number from a Poisson distribution to simulate histogram metrics
			for range numObservations {
				histogram.Observe(GammaRandom(g.rand, 2.0, 2.0))
			}

		case *prometheus.SummaryVec:
			// Update summary values

			summary, err := m.GetMetricWith(prometheus.Labels{})
			if err != nil {
				return err
			}

			numObservations := g.rand.Int() % 10

			// Generate a random number from a Poisson distribution to simulate histogram metrics
			for range numObservations {
				summary.Observe(g.rand.ExpFloat64())
			}

		default:
			return fmt.Errorf("unsupported metric type for %s", name)
		}
	}

	return nil
}

// ServeHTTP implements http.Handler
func (g *Generator) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	format := r.Header.Get("Accept")
	log.Printf("receiver %s request\n", format)
	switch format {
	case "application/json":
		g.serveJSON(w, r)
	case "application/vnd.google.protobuf":
		g.serveProtobuf(w, r)
	default:
		promhttp.HandlerFor(g.registry, promhttp.HandlerOpts{}).ServeHTTP(w, r)
	}
}

// serveJSON serves metrics in JSON format
func (g *Generator) serveJSON(w http.ResponseWriter, r *http.Request) {
	metrics, err := g.registry.Gather()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

// serveProtobuf serves metrics in Protobuf format
func (g *Generator) serveProtobuf(w http.ResponseWriter, r *http.Request) {
	metrics, err := g.registry.Gather()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, metric := range metrics {
		data, err := proto.Marshal(metric)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.google.protobuf")
		w.Write(data)
	}
}

// GammaRandom generates a random number from a Gamma distribution
// shape (k) and scale (theta) are the parameters
func GammaRandom(rand *rand.Rand, shape, scale float64) float64 {
	// Implementation of Marsaglia and Tsang's method
	if shape < 1 {
		// Use transformation for shape < 1
		return GammaRandom(rand, shape+1, scale) * math.Pow(rand.Float64(), 1.0/shape)
	}

	d := shape - 1.0/3.0
	c := 1.0 / math.Sqrt(9.0*d)

	for {
		x := 0.0
		v := 0.0
		for {
			x = rand.NormFloat64()
			v = 1.0 + c*x
			if v > 0 {
				break
			}
		}

		v = v * v * v
		u := rand.Float64()

		if u < 1.0-0.331*math.Pow(x, 4) {
			return d * v * scale
		}

		if math.Log(u) < 0.5*x*x+d*(1.0-v+math.Log(v)) {
			return d * v * scale
		}
	}
}
