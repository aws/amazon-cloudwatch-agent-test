//go:build !windows

package performancetest

import (
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
)

const (
	configOutputPath    = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"
	agentRuntimeMinutes = 5 //20 mins desired but 5 mins for testing purposes
	DynamoDBDataBase    = "CWAPerformanceMetrics"
	testLogNum          = "PERFORMANCE_NUMBER_OF_LOGS"
)

func TestPerformance(t *testing.T) {
	err := common.GenerateLogConfig(logNum, "./config.json")
	if err != nil {
		t.Fatalf("Error: %v", err)
	}

	tpsVals := []int{
		10,
		100,
		1000,
	}

	//data base
	dynamoDB := InitializeTransmitterAPI(DynamoDBDataBase) //add cwa version here
	if dynamoDB == nil {
		t.Fatalf("Error: generating dynamo table")
	}

	//run tests
	for _, tps := range tpsVals {
		t.Run(fmt.Sprintf("TPS run: %d", tps), func(t *testing.T) {
			common.CopyFile(configFilePath, configOutputPath)

			common.StartAgent(configOutputPath, true)

			agentRunDuration := agentRuntimeMinutes * time.Minute

			err := common.StartLogWrite(configFilePath, agentRunDuration, tps)
			if err != nil {
				t.Fatalf("Error: %v", err)
			}

			log.Printf("Agent has been running for : %s\n", (agentRunDuration).String())
			common.StopAgent()

			//collect data
			data, err := GetPerformanceMetrics(instanceId, agentRuntimeMinutes, logNum, tps, agentContext, configFilePath)

			//@TODO check if metrics are zero remove them and make sure there are non-zero metrics existing
			if err != nil {
				t.Fatalf("Error: %v", err)
			}

			if data == nil {
				t.Fatalf("No data")
			}
			// this print shows the sendItem packet,it can be used to debug attribute issues
			fmt.Printf("%v \n", data)

			_, err = dynamoDB.SendItem(data, tps)
			if err != nil {
				t.Fatalf("Error: couldn't upload metric data to table, %v", err)
			}
		})
	}
}

func TestUpdateCommit(t *testing.T) {
	if os.Getenv("IS_RELEASE") != "true" {
		t.SkipNow()
	}
	t.Log("Updating Release Commit", os.Getenv(SHA_ENV))
	dynamoDB := InitializeTransmitterAPI("CWAPerformanceMetrics") //add cwa version here
	releaseHash := os.Getenv(SHA_ENV)
	releaseName := os.Getenv(RELEASE_NAME_ENV)
	if dynamoDB == nil {
		t.Fatalf("Error: generating dynamo table")
		return
	}

	err := dynamoDB.UpdateReleaseTag(releaseHash, releaseName)
	if err != nil {
		t.Fatalf("Error: %s", err)
	}
}
