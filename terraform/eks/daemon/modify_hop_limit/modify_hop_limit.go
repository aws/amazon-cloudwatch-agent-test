// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"log"
	"os"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

func main() {
	args := os.Args
	cluster := args[1]
	hopLimit, err := strconv.Atoi(args[2])
	if err != nil {
		log.Fatalf("could not parse hop limit %s", args[2])
	}
	cxt := context.Background()
	defaultConfig, err := config.LoadDefaultConfig(cxt)
	if err != nil {
		log.Fatal("could not create aws sdk v2 config")
	}
	ec2client := ec2.NewFromConfig(defaultConfig)
	clusterNameFilter := types.Filter{Name: aws.String("tag:eks:cluster-name"), Values: []string{
		cluster,
	}}
	instanceInput := ec2.DescribeInstancesInput{
		Filters: []types.Filter{
			clusterNameFilter,
			{Name: aws.String("instance-state-name"),
				Values: []string{"running"}}}}
	instanceOutput, err := ec2client.DescribeInstances(cxt, &instanceInput)
	if err != nil {
		log.Fatalf("could not get instances for input %v %v", instanceInput, err)
	}
	for _, reservation := range instanceOutput.Reservations {
		for _, instance := range reservation.Instances {
			modifyInstanceMetadataOptionsInput := ec2.ModifyInstanceMetadataOptionsInput{
				InstanceId:              instance.InstanceId,
				HttpPutResponseHopLimit: aws.Int32(int32(hopLimit)),
			}
			_, err := ec2client.ModifyInstanceMetadataOptions(cxt, &modifyInstanceMetadataOptionsInput)
			if err != nil {
				log.Fatalf("could not modify hop limit for instance %v %v", *instance.InstanceId, err)
			}
		}
	}
}
