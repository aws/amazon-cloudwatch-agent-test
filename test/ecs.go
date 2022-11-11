// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build integration
// +build integration

package test

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecs"
)

func RestartDaemonService(clusterArn *string, serviceName *string) error {
	return RestartService(clusterArn, nil, serviceName)
}

func RestartService(clusterArn *string, desiredCount *int64, serviceName *string) error {
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	svc := ecs.New(sess)

	forceNewDeployment := true

	updateServiceInput := &ecs.UpdateServiceInput{
		Cluster:            clusterArn,
		Service:            serviceName,
		ForceNewDeployment: &forceNewDeployment,
	}
	if desiredCount != nil {
		updateServiceInput = updateServiceInput.SetDesiredCount(*desiredCount)
	}

	_, err := svc.UpdateService(updateServiceInput)

	return err
}
