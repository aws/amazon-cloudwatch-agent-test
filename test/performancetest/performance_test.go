//go:build !windows

package performancetest

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/internal/common"
)

const (
	configOutputPath    = "/opt/aws/amazon-cloudwatch-agent/bin/config.json"
	agentRuntimeMinutes = 5 //20 mins desired but 5 mins for testing purposes
	DynamoDBDataBase    = "CWAPerformanceMetrics"
	testLogNum          = "PERFORMANCE_NUMBER_OF_LOGS"
)

var envMetaDataStrings = &(environment.MetaDataStrings{})

func init() {
	environment.RegisterEnvironmentMetaDataFlags(envMetaDataStrings)
}

// this struct is derived from plugins/inputs/logfile FileConfig struct
type LogInfo struct {
	FilePath      string `json:"file_path"`
	LogGroupName  string `json:"log_group_name"`
	LogStreamName string `json:"log_stream_name"`
	Timezone      string `json:"timezone"`
}

func TestPerformance(t *testing.T) {
	//get number of logs for test from github action
	//@TODO
	logNum, err := strconv.Atoi(os.Getenv(testLogNum))
	if err != nil {
		t.Fatalf("Error: cannot convert test log number to integer, %v", err)
	}

	agentContext := context.TODO()
	instanceId := awsservice.GetInstanceId()
	log.Printf("Instance ID used for performance metrics : %s\n", instanceId)

	configFilePath, logStreams, err := GenerateConfig(logNum)
	if err != nil {
		t.Fatalf("Error: %v", err)
	}

	//defer deleting log group and streams
	//defer deleting log group first because golang handles defers in LIFO order
	//and we want to delete the log group after deleting the log streams
	defer awsservice.DeleteLogGroup(instanceId)

	for _, logStream := range logStreams {
		defer awsservice.DeleteLogStream(instanceId, logStream)
	}

	log.Printf("config generated at %s\n", configFilePath)
	defer os.Remove(configFilePath)

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

/* GenerateConfig takes the number of logs to be monitored and applies it to a default config (at ./resources/config.json)
* it writes logs to be monitored of the form /tmp/testNUM.log where NUM is from 1 to number of logs requested to
* ./resources/configNUM.json where NUM is number of logs
* DEFAULT CONFIG MUST BE SUPPLIED WITH AT LEAST ONE LOG BEING MONITORED
* (log being monitored will be overwritten - it is needed for json structure)
* returns the path of the config generated and a list of log stream names
 */
func GenerateConfig(logNum int) (string, []string, error) {
	var cfgFileData map[string]interface{}

	//use default config (for metrics, structure, etc)
	file, err := os.ReadFile("./resources/config.json")
	if err != nil {
		return "", nil, err
	}

	err = json.Unmarshal(file, &cfgFileData)
	if err != nil {
		return "", nil, err
	}

	var logFiles []LogInfo
	var logStreams []string

	for i := 0; i < logNum; i++ {
		logStream := fmt.Sprintf("{instance_id}/tmp%d", i+1)

		logFiles = append(logFiles, LogInfo{
			FilePath:      fmt.Sprintf("/tmp/test%d.log", i+1),
			LogGroupName:  "{instance_id}",
			LogStreamName: logStream,
			Timezone:      "UTC",
		})

		logStreams = append(logStreams, logStream)
	}

	log.Printf("Writing config file with %d logs to ./resources/config%d.json\n", logNum, logNum)

	cfgFileData["logs"].(map[string]interface{})["logs_collected"].(map[string]interface{})["files"].(map[string]interface{})["collect_list"] = logFiles

	finalConfig, err := json.MarshalIndent(cfgFileData, "", " ")
	if err != nil {
		return "", nil, err
	}

	configFilePath := fmt.Sprintf("./resources/config%d.json", logNum)
	f, err := os.Create(configFilePath)
	if err != nil {
		return "", nil, err
	}

	defer f.Close()

	f.Write(finalConfig)

	return configFilePath, logStreams, nil
}

// GetLogFilePaths parses the cloudwatch agent config at the specified path and returns a list of the log files that the
// agent will monitor when using that config file
func GetLogFilePaths(configPath string) ([]string, error) {
	file, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var cfgFileData map[string]interface{}
	err = json.Unmarshal(file, &cfgFileData)
	if err != nil {
		return nil, err
	}

	logFiles := cfgFileData["logs"].(map[string]interface{})["logs_collected"].(map[string]interface{})["files"].(map[string]interface{})["collect_list"].([]interface{})
	var filePaths []string
	for _, process := range logFiles {
		filePaths = append(filePaths, process.(map[string]interface{})["file_path"].(string))
	}

	return filePaths, nil
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
