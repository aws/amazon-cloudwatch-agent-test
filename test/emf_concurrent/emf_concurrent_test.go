// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package emf_concurrent

import (
	"fmt"
	"log"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	testRuntime     = 10 * time.Minute
	threadCount     = 15
	connectionCount = 5
	interval        = 500 * time.Millisecond
	emfAddress      = "0.0.0.0:25888"
)

var (
	// queryString checks that both metric values are the same and have the same expected unit.
	queryString = fmt.Sprintf("filter ispresent(%[1]s) and ispresent(%[2]s) and (%[1]s != %[2]s or (_aws.CloudWatchMetrics.0.Metrics.0.Unit!=%[3]q) or (_aws.CloudWatchMetrics.0.Metrics.1.Unit!=%[3]q))", metricName1, metricName2, metricUnit)
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestConcurrent(t *testing.T) {
	env := environment.GetEnvironmentMetaData()

	common.CopyFile(filepath.Join("testdata", "config.json"), common.ConfigOutputPath)
	require.NoError(t, common.StartAgent(common.ConfigOutputPath, true, false))

	// wait for agent to start up
	time.Sleep(5 * time.Second)

	e := &emitter{
		interval:      interval,
		logGroupName:  fmt.Sprintf("emf-test-group-%s", env.InstanceId),
		logStreamName: fmt.Sprintf("emf-test-stream-%s", env.InstanceId),
		dimension:     env.CwaCommitSha,
		done:          make(chan struct{}),
	}

	defer awsservice.DeleteLogGroup(e.logGroupName)

	tcpAddr, err := net.ResolveTCPAddr("tcp", emfAddress)
	if err != nil {
		log.Fatalf("invalid tcp emfAddress (%s): %v", emfAddress, err)
	}

	var conns []*net.TCPConn
	for i := 0; i < connectionCount; i++ {
		var conn *net.TCPConn
		conn, err = net.DialTCP("tcp", nil, tcpAddr)
		if err != nil {
			log.Fatalf("unable to connect to address (%s): %v", emfAddress, err)
		}
		conns = append(conns, conn)
	}

	log.Printf("Starting EMF emitters for log group (%s)/stream (%s)", e.logGroupName, e.logStreamName)
	startTime := time.Now()
	for i := 0; i < threadCount; i++ {
		e.wg.Add(1)
		go e.start(conns[i%len(conns)])
	}
	time.Sleep(testRuntime)
	close(e.done)
	log.Println("Stopping EMF emitters")
	e.wg.Wait()
	common.StopAgent()
	endTime := time.Now()

	assert.Lenf(t, awsservice.GetLogStreamNames(e.logGroupName), 1, "Detected corruption: multiple streams found")
	log.Printf("Starting query for log group (%s): %s", e.logGroupName, queryString)
	got, err := awsservice.GetLogQueryStats(e.logGroupName, startTime.Unix(), endTime.Unix(), queryString)
	require.NoError(t, err, "Unable to get log query stats")
	assert.NotZero(t, got.RecordsScanned, "No records found in CloudWatch Logs")
	assert.Zerof(t, got.RecordsMatched, "Detected corruption: %v/%v records matched", got.RecordsMatched, got.RecordsScanned)
}
