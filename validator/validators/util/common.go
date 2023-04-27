// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package util

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

func LogCloudWatchDimension(dims []types.Dimension) string {
	var dimension string
	for _, d := range dims {
		if d.Name != nil && d.Value != nil {
			dimension += fmt.Sprintf(" dimension(name=%q, val=%q) ", *d.Name, *d.Value)
		}
	}
	return dimension
}
