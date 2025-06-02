// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows

package logfile

import (
	_ "embed"
)

const tmpConfigPath = "C:\\Users\\Administrator\\AppData\\Local\\Temp\\config.json"

//go:embed resources/config_windows.json
var testConfigJSON string
