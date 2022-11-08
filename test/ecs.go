// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build integration
// +build integration

package test

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecs"
)

func ListTasks(clusterArn *string, serviceName *string) ([]*string, error) {
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	svc := ecs.New(sess)

	listTasksOutput, err := svc.ListTasks(&ecs.ListTasksInput{
		Cluster:     clusterArn,
		ServiceName: serviceName,
	})

	return listTasksOutput.TaskArns, err
}
