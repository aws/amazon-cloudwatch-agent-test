package awsservice

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/google/uuid"
	"log"
	"time"
)

const instanceIdKey = "InstanceId"

func CreateStackName(stackPrefix string) string {
	// Create stack name
	// Only adding the first 5 char for readability
	// Chance of conflict is 1 in 35^5 or about 1 in 52.5 million
	generatedUuid, err := uuid.NewUUID()
	if err != nil {
		log.Fatalf("Failed to create uuid error %v", err)
	}
	return stackPrefix + generatedUuid.String()[0:5]
}

func StartStack(ctx context.Context, stackName string, client *cloudformation.Client, templateText string, timeOutInMinutes int32, parameters []types.Parameter) {
	log.Printf("Template text : %s", templateText)

	// Create cf stack config
	createStackInput := cloudformation.CreateStackInput{
		StackName:        aws.String(stackName),
		TemplateBody:     aws.String(templateText),
		TimeoutInMinutes: aws.Int32(timeOutInMinutes),
		Parameters:       parameters,
	}

	// Start cf stack
	_, err := client.CreateStack(ctx, &createStackInput)
	if err != nil {
		log.Fatalf("Failed to create stack %v", err)
	}
}

func DeleteStack(ctx context.Context, stackName string, client *cloudformation.Client) {
	r := recover()
	deleteStackInput := cloudformation.DeleteStackInput{
		StackName: aws.String(stackName),
	}
	_, err := client.DeleteStack(ctx, &deleteStackInput)
	if err != nil {
		log.Fatalf("Could not delete stack %s", stackName)
	}
	if r != nil {
		log.Fatalf("Delete stack %s is called on panic thus delete stack then continue panic", stackName)
	}
}

func FindStackInstanceId(ctx context.Context, stackName string, client *cloudformation.Client, timeOutMinutes int) string {
	for i := 0; i <= timeOutMinutes; i++ {
		cfStackInput := cloudformation.DescribeStacksInput{
			StackName: aws.String(stackName),
		}
		stacks, err := client.DescribeStacks(ctx, &cfStackInput)
		if err != nil || len(stacks.Stacks) != 1 || stacks.Stacks[0].StackStatus != types.StackStatusCreateComplete {
			log.Printf("Stack %s not ready in minute %d continue to next minute", stackName, i)
		} else {
			for output := range stacks.Stacks[0].Outputs {
				if *stacks.Stacks[0].Outputs[output].OutputKey == instanceIdKey {
					log.Printf("Found instance id %s from stack %s", *stacks.Stacks[0].Outputs[output].OutputValue, stackName)
					return *stacks.Stacks[0].Outputs[output].OutputValue
				}
			}
		}
		log.Printf("Sleep for one minute to wait for stack to start")
		time.Sleep(time.Minute)
	}
	log.Fatalf("Stack not created within timeout %s", stackName)
	return ""
}
