// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"errors"
	"log"

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
	log.Println("hmmmm")
	_, err = dynamodbClient.PutItem(cxt,
		&dynamodb.PutItemInput{
			Item:      item,
			TableName: aws.String(databaseName),
		})

	return err
}

func AddPacketIntoDatabaseIfNotExist(databaseName string, checkingAttribute, checkingAttributeValue []string, packet map[string]interface{}) error {
	item, err := attributevalue.MarshalMap(packet)
	if err != nil {
		return err
	}

	_, err = dynamodbClient.PutItem(cxt,
		&dynamodb.PutItemInput{
			Item:                item,
			TableName:           aws.String(databaseName),
			ConditionExpression: aws.String("#first_attribute <> :first_attribute and #second_attribute <> :second_attribute"),
			ExpressionAttributeNames: map[string]string{
				"#first_attribute":  checkingAttribute[0],
				"#second_attribute": checkingAttribute[1],
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":first_attribute":  &types.AttributeValueMemberN{Value: checkingAttributeValue[0]},
				":second_attribute": &types.AttributeValueMemberS{Value: checkingAttributeValue[1]},
			},
		})

	return err
}

func GetPacketInDatabase(databaseName, indexName string, checkingAttribute, checkingAttributeValue []string, packet map[string]interface{}) (map[string]interface{}, error) {
	var packets []map[string]interface{}
	log.Printf("index name %s %v %v", indexName, checkingAttribute, checkingAttributeValue)
	data, err := dynamodbClient.Query(cxt, &dynamodb.QueryInput{
		TableName:              aws.String(databaseName),
		IndexName:              aws.String(indexName),
		KeyConditionExpression: aws.String("#first_attribute = :first_attribute and #second_attribute = :second_attribute"),
		ExpressionAttributeNames: map[string]string{
			"#first_attribute":  checkingAttribute[0],
			"#second_attribute": checkingAttribute[1],
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":first_attribute":  &types.AttributeValueMemberN{Value: checkingAttributeValue[0]},
			":second_attribute": &types.AttributeValueMemberS{Value: checkingAttributeValue[1]},
		},
		ScanIndexForward: aws.Bool(true), // Sort Range Key in ascending by Sort/Range key in numeric order since range key is CommitDate
	})

	if err != nil {
		return nil, err
	}

	attributevalue.UnmarshalListOfMaps(data.Items, &packets)

	if len(packets) == 0 {
		if packet != nil {
			if err = AddPacketIntoDatabaseIfNotExist(databaseName, checkingAttribute, checkingAttributeValue, packet); err != nil {
				return nil, err
			}
			return packet, nil
		}

		return nil, errors.New("there is no exist package from the database")
	}

	return packets[0], nil
}
