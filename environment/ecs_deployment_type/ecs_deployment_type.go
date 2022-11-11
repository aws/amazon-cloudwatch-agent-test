package ecs_deployment_type

import "strings"

type ECSDeploymentType string

const (
	DAEMON  ECSDeploymentType = "DAEMON"
	REPLICA ECSDeploymentType = "REPLICA"
	SIDECAR ECSDeploymentType = "SIDECAR"
)

var (
	ecsDeploymentTypes = map[string]ECSDeploymentType{
		"DAEMON":  DAEMON,
		"REPLICA": REPLICA,
		"SIDECAR": SIDECAR,
	}
)

func FromString(str string) (ECSDeploymentType, bool) {
	c, ok := ecsDeploymentTypes[strings.ToUpper(str)]
	return c, ok
}
