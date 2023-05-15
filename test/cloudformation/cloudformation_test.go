package cloudformation

import (
	"context"
	"flag"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric"
	"github.com/aws/amazon-cloudwatch-agent-test/test/metric/dimension"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	uuid2 "github.com/google/uuid"
	"log"
	"os"
	"testing"
	"time"
)

const (
	timeOut    = 15
	instanceId = "InstanceId"
	namespace  = "CFMetricTest"
)

var (
	dimensionProviders = []dimension.IProvider{
		&dimension.CustomDimensionProvider{Provider: dimension.Provider{}},
	}
	dimensionFactory = dimension.Factory{Providers: dimensionProviders}
	packagePath      = flag.String("package_path", "", "path to download package")
	iamRole          = flag.String("iam_role", "", "iam role for cf ec2 instance")
	keyName          = flag.String("key_name", "", "key name for ec2 instance")
	metricName       = flag.String("metric_name", "", "metric to look for")
)

func TestCloudformation(t *testing.T) {
	log.Printf("Package path %s iam role %s key name %s metric name %s", *packagePath, *iamRole, *keyName, *metricName)
	cxt := context.Background()
	stackName := createStackName()

	client := createCloudwatchClient(cxt)

	startStack(cxt, stackName, client)

	instanceId := findStackInstanceId(cxt, stackName, client)

	// Sleep so metrics can be added to cloudwatch
	log.Printf("Sleep for one minute to collect metrics")
	time.Sleep(time.Minute)

	dims, failed := dimensionFactory.GetDimensions([]dimension.Instruction{
		{
			Key:   instanceId,
			Value: dimension.ExpectedDimensionValue{Value: aws.String(instanceId)},
		},
	})

	if len(failed) > 0 {
		deleteStack(cxt, stackName, client)
		t.Fatalf("Failed to generate dimensions")
		return
	}

	fetcher := metric.MetricValueFetcher{}
	values, err := fetcher.Fetch(namespace, *metricName, dims, metric.AVERAGE, test_runner.HighResolutionStatPeriod)
	if err != nil {
		deleteStack(cxt, stackName, client)
		t.Fatalf("Failed to find metric %s namespace %s dimension %v", *metricName, namespace, dims)
	}

	if !metric.IsAllValuesGreaterThanOrEqualToExpectedValue(*metricName, values, 0) {
		deleteStack(cxt, stackName, client)
		t.Fatalf("Metric name %s has a negative value", *metricName)
	}

	deleteStack(cxt, stackName, client)
}

func createStackName() string {
	// Create stack name
	uuid, err := uuid2.NewUUID()
	if err != nil {
		log.Fatalf("Failed to create uuid error %v", err)
	}
	stackName := "cfTestStack" + uuid.String()[0:5]
	return stackName
}

func createCloudwatchClient(cxt context.Context) *cloudformation.Client {
	// Create cf client
	cfg, err := config.LoadDefaultConfig(cxt)
	if err != nil {
		log.Fatalf("Can't get config error: %v", err)
	}
	client := cloudformation.NewFromConfig(cfg)
	return client
}

func startStack(cxt context.Context, stackName string, client *cloudformation.Client) {
	// Read template file
	template, err := os.ReadFile("resources/AmazonCloudWatchAgent/inline/amazon_linux.template")
	if err != nil {
		log.Fatalf("Failed to read template file %v", err)
	}
	templateText := string(template)

	log.Printf("Template text : %s", templateText)

	// SCreate cf stack config
	createStackInput := cloudformation.CreateStackInput{
		StackName:        aws.String(stackName),
		TemplateBody:     aws.String(templateText),
		TimeoutInMinutes: aws.Int32(timeOut),
		Parameters: []types.Parameter{
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
		},
	}

	// Start cf stack
	_, err = client.CreateStack(cxt, &createStackInput)
	if err != nil {
		log.Fatalf("Failed to create stack %v", err)
	}
}

func findStackInstanceId(cxt context.Context, stackName string, client *cloudformation.Client) string {
	for i := 0; i <= timeOut; i++ {
		time.Sleep(time.Minute)
		log.Printf("Sleep for one minute to wait for stack to start")
		cfStackInput := cloudformation.DescribeStacksInput{
			StackName: aws.String(stackName),
		}
		stacks, err := client.DescribeStacks(cxt, &cfStackInput)
		if err != nil || len(stacks.Stacks) != 1 || stacks.Stacks[0].StackStatus != types.StackStatusCreateComplete {
			log.Printf("Stack %s not ready in minute %d continue to next minute", stackName, i)
			continue
		}
		for output := range stacks.Stacks[0].Outputs {
			if *stacks.Stacks[0].Outputs[output].OutputKey == instanceId {
				log.Printf("Found instance id %s from stack %s", *stacks.Stacks[0].Outputs[output].OutputValue, stackName)
				return *stacks.Stacks[0].Outputs[output].OutputValue
			}
		}
	}
	deleteStack(cxt, stackName, client)
	log.Fatalf("Stack not created within timeout %s", stackName)
	return ""
}

func deleteStack(cxt context.Context, stackName string, client *cloudformation.Client) {
	deleteStackInput := cloudformation.DeleteStackInput{
		StackName: aws.String(stackName),
	}
	_, err := client.DeleteStack(cxt, &deleteStackInput)
	if err != nil {
		log.Fatalf("Could not delete stack %v", stackName)
	}
}
