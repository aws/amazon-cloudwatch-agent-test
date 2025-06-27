// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows

package windows_event_log

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"sync"

	logstatecommon "github.com/aws/amazon-cloudwatch-agent-test/test/log_state/common"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common/logs"
)

const tmpConfigPath = "C:\\Users\\Administrator\\AppData\\Local\\Temp\\config.json"

//go:embed resources/config.json
var testConfigJSON string

func Validate() error {
	if err := os.WriteFile(tmpConfigPath, []byte(testConfigJSON), 0644); err != nil {
		return fmt.Errorf("could not create config file: %w", err)
	}
	return logstatecommon.Validate(new(generatorStarter), tmpConfigPath)
}

type generatorStarter struct {
	generator *logs.Generator
}

var _ logstatecommon.GeneratorStarter = (*generatorStarter)(nil)

func (g *generatorStarter) Init(_ string) error {
	writer := &windowsEventWriter{
		eventLogName:  "Application",
		eventLogLevel: "INFORMATION",
	}
	generator, err := logs.NewGenerator(&logs.GeneratorConfig{
		LinesPerSecond:  1,
		LineLength:      50,
		TimestampFormat: "2006-01-02T15:04:05.000",
	}, writer)
	if err != nil {
		return fmt.Errorf("could not create logs generator: %v", err)
	}
	g.generator = generator
	return nil
}

func (g *generatorStarter) Start(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(1)
	go g.generator.Generate(ctx, wg)
}

type windowsEventWriter struct {
	eventLogName  string
	eventLogLevel string
}

var _ logs.EntryWriter = (*windowsEventWriter)(nil)

func (w windowsEventWriter) Write(entry string) error {
	return common.CreateWindowsEvent(w.eventLogName, w.eventLogLevel, "1", entry)
}
