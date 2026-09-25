// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package common

import (
	"fmt"
	"log"
	"os"
)

// ReadAgentLogfile returns the contents of the file at logfile. It is OS-agnostic (os.ReadFile is
// platform-neutral), so it lives here rather than being duplicated per build tag.
func ReadAgentLogfile(logfile string) string {
	out, err := os.ReadFile(logfile)
	if err != nil {
		log.Fatal(fmt.Sprint(err) + string(out))
	}
	return string(out)
}
