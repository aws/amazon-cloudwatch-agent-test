// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package xray

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-xray-sdk-go/xray"

	"github.com/aws/amazon-cloudwatch-agent-test/test/trace/generator"
)

var testErr = errors.New("test error")

type Generator struct {
	cfg                     *generator.Config
	testCasesGeneratedCount int
	testCasesEndedCount     int
	done                    chan struct{}
}

func NewLoadGenerator(cfg *generator.Config) generator.Generator {
	return &Generator{
		cfg:                     cfg,
		done:                    make(chan struct{}),
		testCasesGeneratedCount: 0,
		testCasesEndedCount:     0,
	}
}
func (g *Generator) GetTestCount() (int, int) {
	return g.testCasesGeneratedCount, g.testCasesEndedCount
}
func (g *Generator) generate(ctx context.Context) error {
	rootCtx, root := xray.BeginSegment(ctx, "load-generator")
	g.testCasesGeneratedCount++
	defer func() {
		root.Close(nil)
		g.testCasesEndedCount++
	}()

	for key, value := range g.cfg.Annotations {
		if err := root.AddAnnotation(key, value); err != nil {
			return err
		}
	}

	for namespace, metadata := range g.cfg.Metadata {
		for key, value := range metadata {
			if err := root.AddMetadataToNamespace(namespace, key, value); err != nil {
				return err
			}
		}
	}

	_, subSeg := xray.BeginSubsegment(rootCtx, "with-error")
	defer subSeg.Close(nil)

	if err := subSeg.AddError(testErr); err != nil {
		return err
	}

	return nil
}

func (g *Generator) Start(ctx context.Context) error {
	ticker := time.NewTicker(g.cfg.Interval)
	for {
		select {
		case <-g.done:
			ticker.Stop()
			return nil
		case <-ticker.C:
			if err := g.generate(ctx); err != nil {
				return err
			}
		}
	}
}

func (g *Generator) Stop() {
	close(g.done)
}
