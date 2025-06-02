// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package logfile

import (
	_ "embed"
)

const tmpConfigPath = "/tmp/config.json"

//go:embed resources/config_unix.json
var testConfigJSON string
