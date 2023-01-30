// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type dnmdbAPI interface {
	AddPacketIntoDatabase(databaseName, conditionExpression string, packet interface{}, expressAttributesNames map[string]string) error
	GetPacketInDatabase(databaseName, databaseIndexName, conditionExpression string, expressionAttributesNames map[string]string, expressionAttributesValues map[string]types.AttributeValue) ([]map[string]interface{}, error)
	UpdatePacketInDatabase(databaseName, updateExpression, conditionExpression string, expressionAttributesNames map[string]string, expressionAttributesValues, databaseKey map[string]types.AttributeValue) error
}

type dynamodbSDK struct {
	cxt            context.Context
	dynamodbClient *dynamodb.Client
}

func NewDynamoDBSDKClient(cfg aws.Config, cxt context.Context) dnmdbAPI {
	dynamodbClient := dynamodb.NewFromConfig(cfg)
	return &dynamodbSDK{
		cxt:            cxt,
		dynamodbClient: dynamodbClient,
	}
}

func (d *dynamodbSDK) AddPacketIntoDatabase(databaseName, conditionExpression string, packet interface{}, expressAttributesNames map[string]string) error {
	item, err := attributevalue.MarshalMap(packet)
	if err != nil {
		return err
	}

	_, err = d.dynamodbClient.PutItem(d.cxt,
		&dynamodb.PutItemInput{
			Item:                     item,
			TableName:                aws.String(databaseName),
			ConditionExpression:      aws.String(conditionExpression),
			ExpressionAttributeNames: expressAttributesNames,
		})

	return err
}

func (d *dynamodbSDK) GetPacketInDatabase(databaseName, databaseIndexName, conditionExpression string, expressionAttributesNames map[string]string, expressionAttributesValues map[string]types.AttributeValue) ([]map[string]interface{}, error) {
	var packets []map[string]interface{}

	data, err := d.dynamodbClient.Query(d.cxt, &dynamodb.QueryInput{
		TableName:                 aws.String(databaseName),
		IndexName:                 aws.String(databaseIndexName),
		KeyConditionExpression:    aws.String(conditionExpression),
		ExpressionAttributeNames:  expressionAttributesNames,
		ExpressionAttributeValues: expressionAttributesValues,

		ScanIndexForward: aws.Bool(true), // true or false to sort by "date" Sort/Range key ascending or descending
	})
	if err != nil {
		return nil, err
	}

	attributevalue.UnmarshalListOfMaps(data.Items, &packets)
	return packets, nil
}

func (d *dynamodbSDK) UpdatePacketInDatabase(databaseName, updateExpression, conditionExpression string, expressionAttributesNames map[string]string, expressionAttributesValues, databaseKey map[string]types.AttributeValue) error {
	_, err := d.dynamodbClient.UpdateItem(d.cxt, &dynamodb.UpdateItemInput{
		TableName:                 aws.String(databaseName),
		Key:                       databaseKey,
		UpdateExpression:          aws.String(updateExpression),
		ExpressionAttributeNames:  expressionAttributesNames,
		ExpressionAttributeValues: expressionAttributesValues,
		ConditionExpression:       aws.String(conditionExpression),
	})

	return err
}
