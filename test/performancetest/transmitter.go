// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package performancetest

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	backoff "github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"
)

type TransmitterAPI struct {
	DataBaseName string // this is the name of the table when test is run
}

/*
InitializeTransmitterAPI
Desc: Initializes the transmitter class
Side effects: Creates a dynamodb table if it doesn't already exist
*/
func InitializeTransmitterAPI(DataBaseName string) *TransmitterAPI {
	transmitter := TransmitterAPI{
		DataBaseName: DataBaseName,
	}

	return &transmitter
}

/*
PacketMerger()
Desc:

	This function updates the currentPacket with the unique parts of newPacket and returns in dynamo format

Params:

	newPacket: this is the agentData collected in this test
	currentPacket: this is the agentData stored in dynamo currently
*/
func (transmitter *TransmitterAPI) PacketMerger(newPacket map[string]interface{}, currentPacket map[string]interface{}, tps int) (map[string]interface{}, error) {
	testSettings := fmt.Sprintf("%s-%d", os.Getenv(PERFORMANCE_NUMBER_OF_LOGS), tps)
	fmt.Println("The test is", testSettings)
	item := currentPacket[RESULTS].(map[string]interface{})
	_, isPresent := item[testSettings] // check if we already had this test
	if isPresent {
		// we already had this test so ignore it
		return nil, errors.New("nothing to update")
	}
	newAttributes := make(map[string]interface{})
	mergedResults := make(map[string]interface{})
	if newPacket[RESULTS] != nil {
		testSettingValue := newPacket[RESULTS].(map[string]map[string]Stats)[testSettings]
		for attribute, value := range item {
			_, isPresent := newPacket[RESULTS].(map[string]map[string]Stats)[attribute]
			if isPresent {
				continue
			}
			mergedResults[attribute] = value

		}
		mergedResults[testSettings] = testSettingValue
		newAttributes[RESULTS] = mergedResults
	}
	if newPacket[IS_RELEASE] != nil {
		newAttributes[IS_RELEASE] = newPacket[IS_RELEASE]
	}
	if newPacket[HASH] != currentPacket[HASH] {
		newAttributes[HASH] = newPacket[HASH]
	}
	// newAttributes, _ := attributevalue.MarshalMap(mergedResults)
	// newAttributes[IS_RELEASE] = &types.AttributeValueMemberBOOL{Value: true}
	// return newAttributes, nil
	return newAttributes, nil
}

/*
UpdateItem()
Desc:

	This function updates the item in dynamo if the atomic condition is true else it will return ConditionalCheckFailedException

Params:

	hash: this is the commitHash
	targetAttributes: this is the targetAttribute to be added to the dynamo item
	testHash: this is the hash of the last item, used like a version check
*/
func (transmitter *TransmitterAPI) UpdateItem(hash string, packet map[string]interface{}, tps int) error {
	err := backoff.Retry(
		func() error {
			item, err := awsservice.AWS.DnmdbAPI.GetPacketInDatabase(
				transmitter.DataBaseName, "Hash-index", "#hash = :hash",
				map[string]string{
					"#hash": HASH,
				},
				map[string]types.AttributeValue{
					":hash": &types.AttributeValueMemberS{Value: hash},
				},
			) // get most Up to date item from dynamo | O(1) bcs of global sec. idx.

			if len(item) == 0 || err != nil { // check if hash is in dynamo
				return errors.New("ERROR: Hash is not found in dynamo")
			}
			commitDate := fmt.Sprintf("%d", int(item[0][COMMIT_DATE].(float64)))
			year := fmt.Sprintf("%d", int(item[0][PARTITION_KEY].(float64)))
			testHash := item[0][TEST_ID].(string)
			mergedAttributes, err := transmitter.PacketMerger(packet, item[0], tps)
			if err != nil {
				return err
			}
			targetAttributes, err := attributevalue.MarshalMap(mergedAttributes)
			if err != nil {
				return err
			}
			//setup the update expression
			expressionAttributeValues := make(map[string]types.AttributeValue)
			expressionAttributeNames := make(map[string]string)
			expression := "set "
			n_expression := len(targetAttributes)
			i := 0
			for attribute, value := range targetAttributes {
				expressionKey := ":" + strings.ToLower(attribute)
				expressionName := "#" + strings.ToLower(attribute)
				expression += fmt.Sprintf("%s = %s", expressionName, expressionKey)
				expressionAttributeValues[expressionKey] = value
				expressionAttributeNames[expressionName] = attribute
				if n_expression-1 > i {
					expression += ", "
				}
				i++
			}
			expressionAttributeValues[":testID"] = &types.AttributeValueMemberS{Value: testHash}
			expressionAttributeNames["#testID"] = TEST_ID
			//call update
			err = awsservice.AWS.DnmdbAPI.UpdatePacketInDatabase(
				transmitter.DataBaseName,
				expression,
				"#testID = :testID",
				expressionAttributeNames,
				expressionAttributeValues,
				map[string]types.AttributeValue{
					"Year":       &types.AttributeValueMemberN{Value: year},
					"CommitDate": &types.AttributeValueMemberN{Value: commitDate},
				},
			)

			return err
		}, awsservice.StandardExponentialBackoff)

	return err
}

/*
UpdateReleaseTag()
Desc: This function takes in a commit hash and updates the release value to true
Param: commit hash in terms of string
*/
func (transmitter *TransmitterAPI) UpdateReleaseTag(hash string, tagName string) error {
	var err error
	packet := make(map[string]interface{})
	packet[HASH] = tagName
	packet[IS_RELEASE] = true
	packet[TEST_ID] = uuid.New().String()
	err = transmitter.UpdateItem(hash, packet, 0) //try to update the item
	//this may be overwritten by other test threads, in that case it will return a specific error
	if err != nil {
		return err
	}
	return err
}
