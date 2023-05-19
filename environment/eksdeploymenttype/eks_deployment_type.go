// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package eksdeploymenttype

import "strings"

type EKSDeploymentType string

const (
	DAEMON  EKSDeploymentType = "DAEMON"
	REPLICA EKSDeploymentType = "REPLICA"
	SIDECAR EKSDeploymentType = "SIDECAR"
)

var (
	ecsDeploymentTypes = map[string]EKSDeploymentType{
		"DAEMON":  DAEMON,
		"REPLICA": REPLICA,
		"SIDECAR": SIDECAR,
	}
)

func FromString(str string) (EKSDeploymentType, bool) {
	c, ok := ecsDeploymentTypes[strings.ToUpper(str)]
	return c, ok
}
