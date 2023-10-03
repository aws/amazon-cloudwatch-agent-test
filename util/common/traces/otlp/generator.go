package otlp

import (
	"context"
	"errors"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/util/common/traces/base"
	"go.opentelemetry.io/contrib/propagators/aws/xray"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/maps"
)

var generatorError = errors.New("Generator error")

const (
	serviceName                    = "load-generator"
	attributeKeyAwsXrayAnnotations = "aws.xray.annotations"
)

type OtlpTracesGenerator struct {
	base.TraceGenerator
	base.TraceGeneratorInterface
}

func (g *OtlpTracesGenerator) StartSendingTraces(ctx context.Context) error {
	client, shutdown, err := setupClient(ctx)
	if err != nil {
		return err
	}
	defer shutdown(ctx)
	ticker := time.NewTicker(g.Cfg.Interval)
	for {
		select {
		case <-g.Done:
			ticker.Stop()
			return client.ForceFlush(ctx)
		case <-ticker.C:
			if err = g.Generate(ctx); err != nil {
				return err
			}
		}
	}
}
func (g *OtlpTracesGenerator) StopSendingTraces() {
	close(g.Done)
}
func NewLoadGenerator(cfg *base.TraceGeneratorConfig) *OtlpTracesGenerator {
	return &OtlpTracesGenerator{
		TraceGenerator: base.TraceGenerator{
			Cfg:                     cfg,
			Done:                    make(chan struct{}),
			SegmentsGenerationCount: 0,
			SegmentsEndedCount:      0,
		},
	}
}
func (g *OtlpTracesGenerator) Generate(ctx context.Context) error {
	tracer := otel.Tracer("tracer")
	g.SegmentsGenerationCount++
	_, span := tracer.Start(ctx, "example-span", trace.WithSpanKind(trace.SpanKindServer))
	defer func() {
		span.End()
		g.SegmentsEndedCount++
	}()

	if len(g.Cfg.Annotations) > 0 {
		span.SetAttributes(attribute.StringSlice(attributeKeyAwsXrayAnnotations, maps.Keys(g.Cfg.Annotations)))
	}
	span.SetAttributes(g.Cfg.Attributes...)
	return nil
}

func (g *OtlpTracesGenerator) GetSegmentCount() (int, int) {
	return g.SegmentsGenerationCount, g.SegmentsEndedCount
}

func (g *OtlpTracesGenerator) GetAgentConfigPath() string {
	return g.AgentConfigPath
}
func (g *OtlpTracesGenerator) GetAgentRuntime() time.Duration {
	return g.AgentRuntime
}
func (g *OtlpTracesGenerator) GetName() string {
	return g.Name
}
func (g *OtlpTracesGenerator) GetGeneratorConfig() *base.TraceGeneratorConfig {
	return g.Cfg
}

func setupClient(ctx context.Context) (*sdktrace.TracerProvider, func(context.Context) error, error) {
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
	)

	tp, err := setupTraceProvider(ctx, res)
	if err != nil {
		return nil, nil, err
	}

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(xray.Propagator{})

	return tp, func(context.Context) (err error) {
		timeoutCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()

		err = tp.Shutdown(timeoutCtx)
		if err != nil {
			return err
		}
		return nil
	}, nil
}

func setupTraceProvider(ctx context.Context, res *resource.Resource) (*sdktrace.TracerProvider, error) {
	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	return sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithIDGenerator(xray.NewIDGenerator()),
	), nil
}
