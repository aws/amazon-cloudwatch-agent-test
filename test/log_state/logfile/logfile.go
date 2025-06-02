// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package logfile

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	logstatecommon "github.com/aws/amazon-cloudwatch-agent-test/test/log_state/common"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common/logs"
)

func Validate() error {
	if err := os.WriteFile(tmpConfigPath, []byte(testConfigJSON), 0644); err != nil {
		return fmt.Errorf("could not create config file: %w", err)
	}
	return logstatecommon.Validate(new(generatorStarter), tmpConfigPath)
}

type generatorStarter struct {
	generators []*logs.Generator
}

func (g *generatorStarter) Init(configFilePath string) error {
	paths, err := common.GetLogFilePaths(configFilePath)
	if err != nil {
		return err
	}
	for _, path := range paths {
		writer, err := newFileWriter(path)
		if err != nil {
			return err
		}
		generator, err := logs.NewGenerator(&logs.GeneratorConfig{
			LinesPerSecond:  1,
			LineLength:      50,
			TimestampFormat: "2006-01-02T15:04:05.000",
		}, writer)
		if err != nil {
			return err
		}
		g.generators = append(g.generators, generator)
	}
	return nil
}

func (g *generatorStarter) Start(ctx context.Context, wg *sync.WaitGroup) {
	for _, generator := range g.generators {
		wg.Add(1)
		go generator.Generate(ctx, wg)
	}
}

var _ logstatecommon.GeneratorStarter = (*generatorStarter)(nil)

type fileWriter struct {
	file *os.File
	mu   sync.Mutex
}

var _ logs.EntryWriter = (*fileWriter)(nil)

func newFileWriter(filePath string) (*fileWriter, error) {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directories: %w", err)
	}
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	return &fileWriter{file: f}, nil
}

func (w *fileWriter) Write(entry string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, err := w.file.WriteString(entry)
	return err
}

func (w *fileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Close()
}
