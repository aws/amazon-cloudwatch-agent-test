package ecs_launch_type

import "strings"

type ECSLaunchType string

const (
	EC2     ECSLaunchType = "EC2"
	FARGATE ECSLaunchType = "FARGATE"
)

var (
	ecsLaunchTypes = map[string]ECSLaunchType{
		"EC2":     EC2,
		"FARGATE": FARGATE,
	}
)

func FromString(str string) (ECSLaunchType, bool) {
	c, ok := ecsLaunchTypes[strings.ToUpper(str)]
	return c, ok
}
