// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func AddItemIntoDatabase(databaseName, conditionExpression string, packet interface{}, expressAttributesNames map[string]string) error {
	item, err := attributevalue.MarshalMap(packet)
	if err != nil {
		return err
	}

	_, err = dynamodbClient.PutItem(cxt,
		&dynamodb.PutItemInput{
			Item:                     item,
			TableName:                aws.String(databaseName),
			ConditionExpression:      aws.String(conditionExpression),
			ExpressionAttributeNames: expressAttributesNames,
		})

	return err
}

func GetItemInDatabase(databaseName, databaseIndexName, conditionExpression string, expressionAttributesNames map[string]string, expressionAttributesValues map[string]types.AttributeValue) ([]map[string]interface{}, error) {
	var packets []map[string]interface{}

	data, err := dynamodbClient.Query(cxt, &dynamodb.QueryInput{
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

func UpdateItemInDatabase(databaseName, updateExpression, conditionExpression string, expressionAttributesNames map[string]string, expressionAttributesValues, databaseKey map[string]types.AttributeValue) error {
	_, err := dynamodbClient.UpdateItem(cxt, &dynamodb.UpdateItemInput{
		TableName:                 aws.String(databaseName),
		Key:                       databaseKey,
		UpdateExpression:          aws.String(updateExpression),
		ExpressionAttributeNames:  expressionAttributesNames,
		ExpressionAttributeValues: expressionAttributesValues,
		ConditionExpression:       aws.String(conditionExpression),
	})

	return err
}
