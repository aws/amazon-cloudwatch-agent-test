package otlp

import (
	"context"
	"errors"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
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
	common.TracesTestInterface
	cfg                     *common.TraceConfig
	testCasesGeneratedCount int
	testCasesEndedCount     int
	agentConfigPath         string
	agentRuntime            time.Duration
	name                    string
	done                    chan struct{}
}

func (g *OtlpTracesGenerator) StartSendingTraces(ctx context.Context) error {
	client, shutdown, err := setupClient(ctx)
	if err != nil {
		return err
	}
	defer shutdown(ctx)
	ticker := time.NewTicker(g.cfg.Interval)
	for {
		select {
		case <-g.done:
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
	close(g.done)
}
func newLoadGenerator(cfg *common.TraceConfig) *OtlpTracesGenerator {
	return &OtlpTracesGenerator{
		cfg:                     cfg,
		done:                    make(chan struct{}),
		testCasesGeneratedCount: 0,
		testCasesEndedCount:     0,
	}
}
func (g *OtlpTracesGenerator) Generate(ctx context.Context) error {
	tracer := otel.Tracer("tracer")
	g.testCasesGeneratedCount++
	_, span := tracer.Start(ctx, "example-span", trace.WithSpanKind(trace.SpanKindServer))
	defer func() {
		span.End()
		g.testCasesEndedCount++
	}()

	if len(g.cfg.Annotations) > 0 {
		span.SetAttributes(attribute.StringSlice(attributeKeyAwsXrayAnnotations, maps.Keys(g.cfg.Annotations)))
	}
	span.SetAttributes(g.cfg.Attributes...)
	return nil
}

func (g *OtlpTracesGenerator) GetTestCount() (int, int) {
	return g.testCasesGeneratedCount, g.testCasesEndedCount
}

func (g *OtlpTracesGenerator) GetAgentConfigPath() string {
	return g.agentConfigPath
}
func (g *OtlpTracesGenerator) GetAgentRuntime() time.Duration {
	return g.agentRuntime
}
func (g *OtlpTracesGenerator) GetName() string {
	return g.name
}
func (g *OtlpTracesGenerator) GetGeneratorConfig() *common.TraceConfig {
	return g.cfg
}

//func (g *OtlpTracesGenerator) validateSegments(t *testing.T, segments []types.Segment, cfg *common.TraceConfig) {
//	t.Helper()
//	for _, segment := range segments {
//		var result map[string]interface{}
//		require.NoError(t, json.Unmarshal([]byte(*segment.Document), &result))
//		if _, ok := result["parent_id"]; ok {
//			// skip subsegments
//			continue
//		}
//		annotations, ok := result["annotations"]
//		assert.True(t, ok, "missing annotations")
//		assert.True(t, reflect.DeepEqual(annotations, cfg.Annotations), "mismatching annotations")
//		metadataByNamespace, ok := result["metadata"].(map[string]interface{})
//		assert.True(t, ok, "missing metadata")
//		for namespace, wantMetadata := range cfg.Metadata {
//			var gotMetadata map[string]interface{}
//			gotMetadata, ok = metadataByNamespace[namespace].(map[string]interface{})
//			assert.Truef(t, ok, "missing metadata in namespace: %s", namespace)
//			for key, wantValue := range wantMetadata {
//				var gotValue interface{}
//				gotValue, ok = gotMetadata[key]
//				assert.Truef(t, ok, "missing expected metadata key: %s", key)
//				assert.Truef(t, reflect.DeepEqual(gotValue, wantValue), "mismatching values for key (%s):\ngot\n\t%v\nwant\n\t%v", key, gotValue, wantValue)
//			}
//		}
//	}
//}

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
