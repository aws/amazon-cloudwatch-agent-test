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

func AddPacketIntoDatabaseIfNotExist(databaseName, checkingAttribute string, packet map[string]interface{}) error {
	item, err := attributevalue.MarshalMap(packet)
	if err != nil {
		return err
	}

	_, err = dynamodbClient.PutItem(cxt,
		&dynamodb.PutItemInput{
			Item:                item,
			TableName:           aws.String(databaseName),
			ConditionExpression: aws.String("attribute_not_exists(#attribute)"),
			ExpressionAttributeNames: map[string]string{
				"#attribute": checkingAttribute,
			},
		})

	return err
}

func GetPacketInDatabase(databaseName, checkingAttribute, checkingAttributeValue string, packet map[string]interface{}) (map[string]interface{}, error) {
	var packets []map[string]interface{}

	data, err := dynamodbClient.Query(cxt, &dynamodb.QueryInput{
		TableName:              aws.String(databaseName),
		KeyConditionExpression: aws.String("#attribute = :attribute_value"),
		ExpressionAttributeNames: map[string]string{
			"#attribute": checkingAttribute,
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":attribute_value": &types.AttributeValueMemberS{Value: checkingAttributeValue},
		},
		ScanIndexForward: aws.Bool(true), // Sort Range Key in ascending by Sort/Range key in numeric order since range key is CommitDate
	})

	if err != nil {
		return nil, err
	}

	attributevalue.UnmarshalListOfMaps(data.Items, &packets)

	if len(packets) == 0 {
		if packet != nil {
			if err = AddPacketIntoDatabaseIfNotExist(databaseName, checkingAttribute, packet); err != nil {
				return nil, err
			}
			return packet, nil
		}

		return nil, errors.New("there is no exist package from the database")
	}

	return packets[0], nil
}

func UpdatePacketInDatabase() error {
	return nil
}
