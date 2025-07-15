package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

func main() {
	endpoint := flag.String("endpoint", "http://localhost:4318", "OTLP HTTP endpoint")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		<-sigChan
		log.Println("Shutting down gracefully...")
		cancel()
	}()

	if err := run(ctx, *endpoint); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, endpoint string) error {
	// Create resource
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("metrics-generator"),
			attribute.String("test", "histograms"),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to create resource: %v", err)
	}

	// Create OTLP exporter
	exporter, err := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpoint(endpoint),
		otlpmetrichttp.WithInsecure(),
	)
	if err != nil {
		return fmt.Errorf("failed to create exporter: %v", err)
	}

	// Create meter provider
	expView := sdkmetric.NewView(
		sdkmetric.Instrument{
			Name: "test.exponential.histogram",
			Kind: sdkmetric.InstrumentKindHistogram,
		},
		sdkmetric.Stream{
			Name: "test.exponential.histogram",
			Aggregation: sdkmetric.AggregationBase2ExponentialHistogram{
				MaxSize: 160, NoMinMax: false, MaxScale: 20,
			},
		},
	)

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter,
			sdkmetric.WithInterval(10*time.Second),
		)),
		sdkmetric.WithView(expView),
	)
	defer func() {
		if err := meterProvider.Shutdown(ctx); err != nil {
			log.Printf("Failed to shutdown meter provider: %v", err)
		}
	}()

	meter := meterProvider.Meter("metrics-generator")

	// Create instruments
	if err := createAndRecordMetrics(ctx, meter); err != nil {
		return fmt.Errorf("failed to create and record metrics: %v", err)
	}

	<-ctx.Done()
	return nil
}

func createAndRecordMetrics(ctx context.Context, meter metric.Meter) error {
	// Common attributes
	attrs := []attribute.KeyValue{
		attribute.String("test", "histograms"),
	}

	// Counter
	counter, err := meter.Int64Counter("test.counter",
		metric.WithDescription("A test counter that increases by 1"))
	if err != nil {
		return fmt.Errorf("failed to create counter: %v", err)
	}

	// Delta Sum
	deltaSum, err := meter.Int64UpDownCounter("test.delta.sum",
		metric.WithDescription("A test delta sum"))
	if err != nil {
		return fmt.Errorf("failed to create delta sum: %v", err)
	}

	// Cumulative Sum
	cumulativeSum, err := meter.Int64Counter("test.cumulative.sum",
		metric.WithDescription("A test cumulative sum"))
	if err != nil {
		return fmt.Errorf("failed to create cumulative sum: %v", err)
	}

	// Gauge
	gauge, err := meter.Float64ObservableGauge("test.gauge",
		metric.WithDescription("A test gauge"))
	if err != nil {
		return fmt.Errorf("failed to create gauge: %v", err)
	}

	// Histogram
	histogram, err := meter.Float64Histogram("test.histogram",
		metric.WithDescription("A test histogram"))
	if err != nil {
		return fmt.Errorf("failed to create histogram: %v", err)
	}

	// Exponential Histogram
	expHistogram, err := meter.Float64Histogram("test.exponential.histogram",
		metric.WithDescription("A test histogram"))
	if err != nil {
		return fmt.Errorf("failed to create histogram: %v", err)
	}

	// Start a goroutine to update metrics
	go func() {

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Update counter
				counter.Add(ctx, 1, metric.WithAttributes(attrs...))

				// Update delta sum
				deltaSum.Add(ctx, 5, metric.WithAttributes(attrs...))

				// Update cumulative sum
				cumulativeSum.Add(ctx, 10, metric.WithAttributes(attrs...))

				// Update histogram with various values
				histogram.Record(ctx, 5, metric.WithAttributes(attrs...))
				histogram.Record(ctx, 15, metric.WithAttributes(attrs...))
				histogram.Record(ctx, 35, metric.WithAttributes(attrs...))
				histogram.Record(ctx, 75, metric.WithAttributes(attrs...))
				histogram.Record(ctx, 150, metric.WithAttributes(attrs...))

				// Generate exponentially distributed values
				expHistogram.Record(ctx, 0.5, metric.WithAttributes(attrs...))
				expHistogram.Record(ctx, 1, metric.WithAttributes(attrs...))
				expHistogram.Record(ctx, 2, metric.WithAttributes(attrs...))
				expHistogram.Record(ctx, 4, metric.WithAttributes(attrs...))
				expHistogram.Record(ctx, 8, metric.WithAttributes(attrs...))
				expHistogram.Record(ctx, 16, metric.WithAttributes(attrs...))
			}
		}
	}()

	// Register gauge callback
	_, err = meter.RegisterCallback(
		func(_ context.Context, o metric.Observer) error {
			// Simulate an oscillating value
			o.ObserveFloat64(gauge, float64(50+30*math.Sin(float64(time.Now().Unix())/10)))
			return nil
		},
		gauge,
	)
	if err != nil {
		return fmt.Errorf("failed to register gauge callback: %v", err)
	}

	return nil
}
