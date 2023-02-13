// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func ReplacePacketInDatabase(databaseName string, packet map[string]interface{}) error {
	item, err := attributevalue.MarshalMap(packet)
	if err != nil {
		return err
	}

	_, err = dynamodbClient.PutItem(cxt,
		&dynamodb.PutItemInput{
			Item:      item,
			TableName: aws.String(databaseName),
		})

	return err
}

func AddPacketIntoDatabaseIfNotExist(databaseName, useCase, checkingAttribute, checkingAttributeValue string, packet map[string]interface{}) error {
	item, err := attributevalue.MarshalMap(packet)
	if err != nil {
		return err
	}

	_, err = dynamodbClient.PutItem(cxt,
		&dynamodb.PutItemInput{
			Item:                item,
			TableName:           aws.String(databaseName),
			ConditionExpression: aws.String("#usecase <> :usecase_name and #attribute <> :attribute_value"),
			ExpressionAttributeNames: map[string]string{
				"#usecase":   "UseCase",
				"#attribute": checkingAttribute,
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":usecase_name":    &types.AttributeValueMemberS{Value: useCase},
				":attribute_value": &types.AttributeValueMemberN{Value: checkingAttributeValue},
			},
		})

	return err
}

func GetPacketInDatabase(databaseName, useCase, checkingAttribute, checkingAttributeValue string, packet map[string]interface{}) (map[string]interface{}, error) {
	var packets []map[string]interface{}

	data, err := dynamodbClient.Query(cxt, &dynamodb.QueryInput{
		TableName:              aws.String(databaseName),
		KeyConditionExpression: aws.String("#usecase = :usecase_name and #attribute = :attribute_value"),
		ExpressionAttributeNames: map[string]string{
			"#usecase":   "UseCase",
			"#attribute": checkingAttribute,
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":usecase_name":    &types.AttributeValueMemberS{Value: useCase},
			":attribute_value": &types.AttributeValueMemberN{Value: checkingAttributeValue},
		},
		ScanIndexForward: aws.Bool(true), // Sort Range Key in ascending by Sort/Range key in numeric order since range key is CommitDate
	})

	if err != nil {
		return nil, err
	}

	attributevalue.UnmarshalListOfMaps(data.Items, &packets)

	if len(packets) == 0 {
		if packet != nil {
			if err = AddPacketIntoDatabaseIfNotExist(databaseName, useCase, checkingAttribute, checkingAttributeValue, packet); err != nil {
				return nil, err
			}
			return packet, nil
		}

		return nil, errors.New("there is no exist package from the database")
	}

	return packets[0], nil
}
