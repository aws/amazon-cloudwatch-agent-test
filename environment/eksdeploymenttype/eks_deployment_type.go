// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package eksdeploymenttype

import "strings"

type EKSDeploymentType string

const (
	DAEMON      EKSDeploymentType = "DAEMON"
	REPLICA     EKSDeploymentType = "REPLICA"
	SIDECAR     EKSDeploymentType = "SIDECAR"
	STATEFUL    EKSDeploymentType = "STATEFUL"
	PODIDENTITY EKSDeploymentType = "PODIDENTITY"
)

var (
	eksDeploymentTypes = map[string]EKSDeploymentType{
		"DAEMON":      DAEMON,
		"REPLICA":     REPLICA,
		"SIDECAR":     SIDECAR,
		"STATEFUL":    STATEFUL,
		"PODIDENTITY": PODIDENTITY,
	}
)

func FromString(str string) (EKSDeploymentType, bool) {
	c, ok := eksDeploymentTypes[strings.ToUpper(str)]
	return c, ok
}
