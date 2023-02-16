package performancetest

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

const (
	UPDATE_DELAY_THRESHOLD = 60 // this is how long we want to wait for random sleep in seconds
	MAX_ATTEMPTS           = 5  // number of attemps before we stop retrying to update
	/*
		!Warning: if this value is less than 25 there is a risk of testCases being lost.
		This will only happen if all test threads and at the same time and get the same
		sleep value after first attempt to add ITEM
	*/
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
	fmt.Println("API ready")
	return &transmitter

}

/*
AddItem()
Desc: Takes in a packet and
will convert to dynamodb format  and upload to dynamodb table.
Param:

	packet * map[string]interface{}:  is a map with data collection data

Side effects:

	Adds an item to dynamodb table
*/
func (transmitter *TransmitterAPI) AddItem(packet map[string]interface{}) (string, error) {
	var ae *types.ConditionalCheckFailedException // this exception represent the atomic check has failed
	item, err := attributevalue.MarshalMap(packet)
	if err != nil {
		panic(err)
	}
	err = awsservice.AddItemIntoDatabase(transmitter.DataBaseName, "attribute_not_exists(#hash)", packet, map[string]string{"#hash": HASH})

	if err != nil && !errors.As(err, &ae) {
		fmt.Printf("Error adding item to table.  %v\n", err)
	}
	return fmt.Sprintf("%v", item), err
}

/*
SendItem()
Desc: Parses the input data and adds it to the dynamo table
Param: packet map[string]interface{} is the data collected by data collector
*/
func (transmitter *TransmitterAPI) SendItem(packet map[string]interface{}, tps int) (string, error) {
	var sentItem string
	var ae *types.ConditionalCheckFailedException // this exception represent the atomic check has failed
	// check if hash exists
	currentItem, err := transmitter.Query(packet[HASH].(string))
	if err != nil {
		return "", err
	}
	if len(currentItem) == 0 { // if an item with the same hash doesn't exist add it
		sentItem, err = transmitter.AddItem(packet)
		// this may be overwritten by other test threads, in that case it will return a specific error

		if !errors.As(err, &ae) { // check if our add call got overwritten by other threads
			return sentItem, err
		}
		if err != nil { //any other error dont try again
			return "", err
		}
		// addItem failed due to a competing thread
		// instead of adding, proceed to update the item, with the same data
		rand.Seed(time.Now().UnixNano())
		time.Sleep(time.Duration(rand.Intn(UPDATE_DELAY_THRESHOLD)) * time.Second)
		fmt.Println("Item already exist going to update", len(currentItem))
	}
	// item already exist so update the item instead
	err = transmitter.UpdateItem(packet[HASH].(string), packet, tps) //try to update the item
	//this may be overwritten by other test threads, in that case it will return a specific error
	if err != nil {
		return "", err
	}
	fmt.Println("SendItem Completed")
	return sentItem, err

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
		return nil, errors.New("Nothing to update")
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
	var ae *types.ConditionalCheckFailedException // this exception represent the atomic check has failed
	rand.Seed(time.Now().UnixNano())
	randomSleepDuration := time.Duration(rand.Intn(UPDATE_DELAY_THRESHOLD)) * time.Second
	for attemptCount := 0; attemptCount < MAX_ATTEMPTS; attemptCount++ {
		fmt.Println("Updating:", hash)
		item, err := transmitter.Query(hash) // get most Up to date item from dynamo | O(1) bcs of global sec. idx.
		if len(item) == 0 {                  // check if hash is in dynamo
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
		err = awsservice.UpdateItemInDatabase(
			transmitter.DataBaseName, expression,
			"#testID = :testID", expressionAttributeNames, expressionAttributeValues,
			map[string]types.AttributeValue{
				"Year":       &types.AttributeValueMemberN{Value: year},
				"CommitDate": &types.AttributeValueMemberN{Value: commitDate},
			},
		)
		if errors.As(err, &ae) { //check if our call got overwritten
			// item has changed
			fmt.Println("Retrying...")
			time.Sleep(randomSleepDuration)
			continue
		}
		if err != nil {
			return err
		}
		fmt.Println("Update Completed")
		return nil
	}
	// if the code reaches here it means we have reach MAX_ATTEMPTS
	return errors.New("ERROR: We reached max number of attempts dropping the update")
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

func (transmitter *TransmitterAPI) Query(hash string) ([]map[string]interface{}, error) {
	packets, err := awsservice.GetItemInDatabase(
		transmitter.DataBaseName,
		"Hash-index",
		"#hash = :hash",
		map[string]string{
			"#hash": HASH,
		},
		map[string]types.AttributeValue{
			":hash": &types.AttributeValueMemberS{Value: hash},
		},
	)

	if err != nil {
		return nil, err
	}

	return packets, err
}
