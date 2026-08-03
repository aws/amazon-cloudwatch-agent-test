// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build linux

package journald

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	logstatecommon "github.com/aws/amazon-cloudwatch-agent-test/test/log_state/common"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common/logs"
)

const tmpConfigPath = "/tmp/journald_config.json"

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
	writer := &journaldWriter{
		identifier: "cwagent-test",
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

type journaldWriter struct {
	identifier string
}

var _ logs.EntryWriter = (*journaldWriter)(nil)

func (w *journaldWriter) Write(entry string) error {
	cmd := exec.Command("systemd-cat", "--identifier="+w.identifier, "--priority=info")
	cmd.Stdin = strings.NewReader(entry)
	return cmd.Run()
}
