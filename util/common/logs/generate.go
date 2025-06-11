// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package logs

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"golang.org/x/time/rate"
)

const (
	charset          = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	logEntryTemplate = "%s seq=%d %s\n"
)

var (
	seqNumRegex = regexp.MustCompile(`seq=(\d+)`)
)

type EntryWriter interface {
	Write(entry string) error
}

type GeneratorConfig struct {
	LinesPerSecond  int
	LineLength      int
	TimestampFormat string
}

func (c *GeneratorConfig) validate() error {
	if c.LinesPerSecond <= 0 {
		return errors.New("LinesPerSecond must be greater than zero")
	}
	if c.LineLength <= 0 {
		return errors.New("LineLength must be greater than zero")
	}
	if c.TimestampFormat == "" {
		return errors.New("TimestampFormat must be set")
	}
	return nil
}

type Generator struct {
	cfg            *GeneratorConfig
	limiter        *rate.Limiter
	sequenceNumber atomic.Uint64
	writer         EntryWriter
}

func NewGenerator(cfg *GeneratorConfig, writer EntryWriter) (*Generator, error) {
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %v", err)
	}
	if writer == nil {
		return nil, errors.New("writer must not be nil")
	}
	return &Generator{
		cfg:     cfg,
		writer:  writer,
		limiter: rate.NewLimiter(rate.Limit(cfg.LinesPerSecond), cfg.LinesPerSecond),
	}, nil
}

func (g *Generator) Generate(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			if err := g.limiter.Wait(ctx); err != nil {
				continue
			}
			if err := g.writer.Write(g.generateEntry()); err != nil {
				log.Printf("write entry failed: %v", err)
			}
		}
	}
}

func (g *Generator) SequenceNumber() uint64 {
	return g.sequenceNumber.Load()
}

func (g *Generator) generateEntry() string {
	sequenceNumber := g.sequenceNumber.Add(1)
	timestamp := time.Now().Format(g.cfg.TimestampFormat)
	randomContent := generateRandomString(g.cfg.LineLength - len(timestamp) - 20)
	return fmt.Sprintf(logEntryTemplate, timestamp, sequenceNumber, randomContent)
}

func generateRandomString(length int) string {
	if length <= 0 {
		return ""
	}
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

func AssertNoMissingLogs(events []types.OutputLogEvent) error {
	var skipped int
	seqNums := make([]uint64, 0, len(events))
	for _, event := range events {
		match := seqNumRegex.FindStringSubmatch(*event.Message)
		if len(match) != 2 {
			skipped++
			continue
		}
		seqNum, err := strconv.ParseUint(match[1], 10, 64)
		if err != nil {
			skipped++
			continue
		}
		seqNums = append(seqNums, seqNum)
	}
	if skipped > 0 {
		log.Printf("Skipped %d events that didn't have sequence numbers", skipped)
	}
	sort.Slice(seqNums, func(i, j int) bool {
		return seqNums[i] < seqNums[j]
	})
	gotCount := uint64(len(seqNums))
	first := seqNums[0]
	last := seqNums[len(seqNums)-1]
	wantCount := last - first + 1
	if gotCount != wantCount {
		missing := make([]uint64, 0)
		current := first
		for _, seqNum := range seqNums {
			for current < seqNum {
				missing = append(missing, current)
				current++
			}
			current++
		}
		if len(missing) > 0 {
			return fmt.Errorf("missing %d out of %d: %v", len(missing), gotCount, missing)
		}
	}
	return nil
}
