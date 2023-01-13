// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build integration
// +build integration

package awsservice

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

const (
	instanceId = "InstanceId"
	loremIpsum = "Lorem ipsum dolor sit amet consectetur adipiscing elit Vivamus non mauris malesuada mattis ex eget porttitor purus Suspendisse potenti Praesent vel sollicitudin ipsum Quisque luctus pretium lorem non faucibus Ut vel quam dui Nunc fermentum condimentum consectetur Morbi tellus mauris tristique tincidunt elit consectetur hendrerit placerat dui In nulla erat finibus eget erat a hendrerit sodales urna In sapien purus auctor sit amet congue ut congue eget nisi Vivamus sed neque ut ligula lobortis accumsan quis id metus In feugiat velit et leo mattis non fringilla dui elementum Proin a nisi ac sapien vulputate consequat Vestibulum eu tellus mi Integer consectetur efficitur"
)

func BuildDimensionFilterList(appendDimension int) []types.DimensionFilter {
	// we append dimension from 0 to max number - 2
	// then we add dimension instance id
	// thus for max dimension 10, 0 to 8 + instance id = 10 dimension
	ec2InstanceId := GetInstanceId()
	dimensionFilter := make([]types.DimensionFilter, appendDimension)
	for i := 0; i < appendDimension-1; i++ {
		dimensionFilter[i] = types.DimensionFilter{
			Name:  aws.String(string(i)),
			Value: aws.String(fmt.Sprintf("%s%d", loremIpsum, i)),
		}
	}
	dimensionFilter[appendDimension-1] = types.DimensionFilter{
		Name:  aws.String(instanceId),
		Value: aws.String(ec2InstanceId),
	}
	return dimensionFilter
}
