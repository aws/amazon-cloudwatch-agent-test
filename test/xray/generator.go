package xray

import (
	"context"
	"errors"
	"log"
	"os"
	"path"
	"time"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/aws/aws-xray-sdk-go/strategy/sampling"
	"github.com/aws/aws-xray-sdk-go/xray"
	"github.com/aws/aws-xray-sdk-go/xraylog"
)

var generatorError = errors.New("Generator error")
type XrayTracesGenerator struct {
	common.TraceGenerator
	common.TraceGeneratorInterface
}

func (g *XrayTracesGenerator) StartSendingTraces(ctx context.Context) error {
	ticker := time.NewTicker(g.Cfg.Interval)
	for {
		select {
		case <-g.Done:
			ticker.Stop()
			return nil
		case <-ticker.C:
			if err := g.Generate(ctx); err != nil {
				return err
			}
		}
	}
}
func (g *XrayTracesGenerator) StopSendingTraces() {
	close(g.Done)
}
func newLoadGenerator(cfg *common.TraceGeneratorConfig) *XrayTracesGenerator {
	s, err := sampling.NewLocalizedStrategyFromFilePath(
		path.Join("resources", "sampling-rule.json"))
	if err != nil {
		log.Fatalf("Couldn't apply sampling rule : %s", err)
	}
	xray.Configure(xray.Config{SamplingStrategy: s})
	xray.SetLogger(xraylog.NewDefaultLogger(os.Stdout, xraylog.LogLevelWarn))
	return &XrayTracesGenerator{
		TraceGenerator: common.TraceGenerator{
			Cfg:                     cfg,
			Done:                    make(chan struct{}),
			SegmentsGenerationCount: 0,
			SegmentsEndedCount:      0,
		},
	}
}
func (g *XrayTracesGenerator) Generate(ctx context.Context) error {
	rootCtx, root := xray.BeginSegment(ctx, "load-generator")
	g.SegmentsGenerationCount++
	defer func() {
		root.Close(nil)
		g.SegmentsEndedCount++
	}()

	for key, value := range g.Cfg.Annotations {
		if err := root.AddAnnotation(key, value); err != nil {
			return err
		}
	}

	for namespace, metadata := range g.Cfg.Metadata {
		for key, value := range metadata {
			if err := root.AddMetadataToNamespace(namespace, key, value); err != nil {
				return err
			}
		}
	}

	_, subSeg := xray.BeginSubsegment(rootCtx, "with-error")
	defer subSeg.Close(nil)

	if err := subSeg.AddError(generatorError); err != nil {
		return err
	}

	return nil
}

func (g *XrayTracesGenerator) GetSegmentCount() (int, int) {
	return g.SegmentsGenerationCount, g.SegmentsEndedCount
}

func (g *XrayTracesGenerator) GetAgentConfigPath() string {
	return g.AgentConfigPath
}
func (g *XrayTracesGenerator) GetAgentRuntime() time.Duration {
	return g.AgentRuntime
}
func (g *XrayTracesGenerator) GetName() string {
	return g.Name
}
func (g *XrayTracesGenerator) GetGeneratorConfig() *common.TraceGeneratorConfig {
	return g.Cfg
}
