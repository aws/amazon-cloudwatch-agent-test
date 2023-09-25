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
	threadCount     = 10
	connectionCount = 10
	interval        = 500 * time.Millisecond
	emfAddress      = "0.0.0.0:25888"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

func TestConcurrent(t *testing.T) {
	env := environment.GetEnvironmentMetaData()

	common.CopyFile(filepath.Join("testdata", "config.json"), common.ConfigOutputPath)
	require.NoError(t, common.StartAgent(common.ConfigOutputPath, true, false))

	// wait for agent to start up
	time.Sleep(10 * interval)

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
	e.wg.Wait()
	common.StopAgent()
	endTime := time.Now()
	log.Println("Stopping EMF emitters")

	var gotStreamNames []string
	for _, stream := range awsservice.GetLogStreams(e.logGroupName) {
		gotStreamNames = append(gotStreamNames, *stream.LogStreamName)
	}
	assert.Lenf(t, gotStreamNames, 1, "Detected corruption: multiple streams found")
	qs := queryString()
	log.Printf("Starting query for log group (%s): %s", e.logGroupName, qs)
	gotLogQueryStats, err := awsservice.GetLogQueryStats(e.logGroupName, startTime.Unix(), endTime.Unix(), qs)
	require.NoError(t, err, "Unable to get log query stats")
	assert.NotZero(t, gotLogQueryStats.RecordsScanned, "No records found in CloudWatch Logs")
	assert.Zerof(t, gotLogQueryStats.RecordsMatched, "Detected corruption: %v/%v records matched", gotLogQueryStats.RecordsMatched, gotLogQueryStats.RecordsScanned)
}

func queryString() string {
	return fmt.Sprintf("filter ispresent(%[1]s) and ispresent(%[2]s) and (%[1]s != %[2]s or (_aws.CloudWatchMetrics.0.Metrics.0.Unit!=%[3]q) or (_aws.CloudWatchMetrics.0.Metrics.1.Unit!=%[3]q))", metricName1, metricName2, metricUnit)
}
