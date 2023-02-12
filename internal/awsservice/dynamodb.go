// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

func UpdateOrAddPacketInDatabase(databaseName string, packet map[string]interface{}) error {
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
