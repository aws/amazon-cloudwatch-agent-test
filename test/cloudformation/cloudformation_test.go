// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package cloudformation

import (
	"context"
	"flag"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"log"
	"os"
	"testing"
	"time"
)

const (
	timeOutMinutes = 15
	instanceIdKey  = "InstanceId"
	namespace      = "CFMetricTest"
)

var (
	dimensionProviders = []dimension.IProvider{
		&dimension.CustomDimensionProvider{Provider: dimension.Provider{}},
	}
	dimensionFactory = dimension.Factory{Providers: dimensionProviders}
	packagePath      = flag.String("package_path", "", "s3 path to download package")
	iamRole          = flag.String("iam_role", "", "iam role for cf ec2 instance")
	keyName          = flag.String("key_name", "", "key name for ec2 instance")
	metricName       = flag.String("metric_name", "", "metric to look for")
)

func TestCloudformation(t *testing.T) {
	log.Printf("Package path %s iam role %s key name %s metric name %s", *packagePath, *iamRole, *keyName, *metricName)
	ctx := context.Background()
	stackName := awsservice.CreateStackName("cfTestStack")

	client := awsservice.CloudformationClient

	// Read template file
	template, err := os.ReadFile("resources/AmazonCloudWatchAgent/inline/amazon_linux.template")
	if err != nil {
		log.Fatalf("Failed to read template file %v", err)
	}
	templateText := string(template)

	parameters := []types.Parameter{
		{
			ParameterKey:   aws.String("KeyName"),
			ParameterValue: keyName,
		},
		{
			ParameterKey:   aws.String("PackageLocation"),
			ParameterValue: packagePath,
		},
		{
			ParameterKey:   aws.String("MetricNamespace"),
			ParameterValue: aws.String(namespace),
		},
		{
			ParameterKey:   aws.String("IAMRole"),
			ParameterValue: iamRole,
		},
	}
	awsservice.StartStack(ctx, stackName, client, templateText, timeOutMinutes, parameters)
	defer awsservice.DeleteStack(ctx, stackName, client)

	instanceId := awsservice.FindStackInstanceId(ctx, stackName, client, timeOutMinutes)

	// Sleep so metrics can be added to cloudwatch
	log.Printf("Sleep for one minute to collect metrics")
	time.Sleep(time.Minute)

	dims, failed := dimensionFactory.GetDimensions([]dimension.Instruction{
		{
			Key:   instanceIdKey,
			Value: dimension.ExpectedDimensionValue{Value: aws.String(instanceId)},
		},
	})

	if len(failed) > 0 {
		t.Fatalf("Failed to generate dimensions")
		return
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(namespace, *metricName, dims, metric.AVERAGE, metric.HighResolutionStatPeriod)
	if err != nil {
		t.Fatalf("Failed to find metric %s namespace %s dimension %v", *metricName, namespace, dims)
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(*metricName, values, 0) {
		t.Fatalf("Metric name %s has a negative value", *metricName)
	}

}
