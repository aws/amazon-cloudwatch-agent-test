package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path"
	"runtime"
	"strconv"
	"time"
)

const (
	logTruncateSize    = 100 * 1024 * 1024
	structuredLogEvent = `{"Type": "Cluster","Version": "0","TaskCount": 4,"CloudWatchMetrics": [{"Metrics": [{"Unit": "Count","Name": "TaskCount"}, {"Unit": "Count","Name": "ServiceCount"}],"Dimensions": [["ClusterName"]],"Namespace": "IntegrationTest"}],"ClusterName": "cluster-integ-test","Timestamp":%d,"ServiceCount": 2}`
)

var (
	fileNum          = flag.Int("fileNum", 1, "Identify the structuredLogs file count")
	eventsPerSecond  = flag.Int("eventsPerSecond", 65, "Identify the structuredLogs event count per second per file.")
	structuredLogDir = flag.String("path", "", "Identify the directory where the structured log files will be generated.")
	filePrefix       = flag.String("filePrefix", "structuredLogFile", "Identify the structured log file prefix")
	runTime          = flag.Duration("runTime", 48*time.Hour, "Run time duration.")
)

func main() {
	flag.Parse()
	if *structuredLogDir == "" {
		if "windows" == runtime.GOOS {
			*structuredLogDir = "C:\\tmp\\soakTest"
		} else {
			*structuredLogDir = "/tmp/soakTest"
		}
	}
	// Start generating structured log
	for i := 0; i < *fileNum; i++ {
		go writeStructuredLog(i)
	}
	time.Sleep(*runTime)
	// No cleanup needed, just exit.
}

func writeStructuredLog(fileIndex int) {
	eventSize := len(structuredLogEvent) + len(strconv.FormatInt(makeTimestamp(), 10)) - 2
	curFilePath := path.Join(*structuredLogDir, fmt.Sprintf("%s%d.json", *filePrefix, fileIndex))
	fmt.Printf("Creating file %s\n", curFilePath)

	os.MkdirAll(*structuredLogDir, 0755)
	sf, _ := os.Create(curFilePath)
	fileSize := 0
	// add jitter here to ensure multiple stream will not write at same time.
	r := time.Duration(rand.Intn(1000))
	time.Sleep(r * time.Millisecond)
	ticker := time.NewTicker(time.Second)
	for range ticker.C {
		for i := 0; i < *eventsPerSecond; i++ {
			sf.WriteString(fmt.Sprintf(structuredLogEvent+"\n", makeTimestamp()))
		}
		sf.Sync()
		fileSize += (*eventsPerSecond) * (eventSize)
		if fileSize >= logTruncateSize {
			os.Truncate(curFilePath, 0)
			sf.Seek(0, 0)
			fileSize = 0
		}
	}
}

// returns the timeStamp in millisecond
func makeTimestamp() int64 {
	return time.Now().UnixMilli()
}
