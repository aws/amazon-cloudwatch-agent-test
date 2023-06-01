// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package restart

import (
	"fmt"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
)

func LogCheck(cmd string) string {
	var before, after string
	var err error
	before, err = common.RunShellScript(cmd)
	if err != nil {
		return fmt.Sprintf("Running log check script for restart test failed: %v", err)
	}

	time.Sleep(30 * time.Second)

	after, err = common.RunShellScript(cmd)
	if err != nil {
		return fmt.Sprintf("Running log check script for restart test failed: %v", err)
	}

	if before != after {
		return fmt.Sprintf("Logs are flowing, first time the log size is %s, while second time it become %s.", before, after)
	}

	return ""
}
