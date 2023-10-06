// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

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
	layoutFormat    = "02 Jan 06 15:04:05 MST"
	logTruncateSize = 32 * 1024 * 1024
	multilineRatio  = 10 //means every 10 lines will have a multilineStarter(timestamp)
)

var (
	fileNum         = flag.Int("fileNum", 1, "Identify the file count.")
	eventsPerSecond = flag.Int("eventsPerSecond", 200, "Identify the log event count per second per file.")
	eventSize       = flag.Int("eventSize", 120, "Identify the single log line size, in Byte.")
	outPutPath      = flag.String("path", "", "Identify the path where log files will generate.")
	filePrefix      = flag.String("filePrefix", "tmp", "Identify the log file prefix")
	runTime         = flag.Duration("runTime", 48*time.Hour, "Run time duration.")
)

func main() {
	flag.Parse()
	fmt.Printf("Start writing %d files, and each file has throughput %d Bytes/sec...\n",
		*fileNum, (*eventsPerSecond)*(*eventSize))

	if *outPutPath == "" {
		if runtime.GOOS == "windows" {
			*outPutPath = "C:\\tmp\\soakTest"
		} else {
			*outPutPath = "/tmp/soakTest"
		}
	}
	//Create log line string
	logEntry := ""
	for i := 0; i < *eventSize-1; i++ {
		logEntry += "A"
	}
	//Start generating log
	for i := 0; i < *fileNum; i++ {
		go writeLog(logEntry, i)
	}
	time.Sleep(*runTime)
	// No cleanup needed, just exit.
}

func writeLog(logEntry string, instanceId int) {
	curFilePath := path.Join(*outPutPath, fmt.Sprintf("%s%d.log", *filePrefix, instanceId))
	fmt.Printf("Creating file %s\n", curFilePath)
	os.MkdirAll(*outPutPath, 0755)
	f, _ := os.Create(curFilePath)
	fileSize := 0
	// add jitter here to ensure multiple stream will not write at same time.
	r := time.Duration(rand.Intn(1000))
	time.Sleep(r * time.Millisecond)
	ticker := time.NewTicker(time.Second)
	for range ticker.C {
		fmt.Printf("%s Starting...\n", time.Now().Format(layoutFormat))
		for i := 0; i < *eventsPerSecond; i++ {
			if i%multilineRatio == 0 {
				// Add timestamp to the begining of the line as multiline starter and append logEntry[0:len(logEntry)-len(layoutFormat)-1], -1 here is because the extra " " after added timestamp.
				f.WriteString(time.Now().Format(layoutFormat) + " " +
					logEntry[:(len(logEntry)-len(layoutFormat)-1)] + "\n")
			} else {
				instanceIdstr := strconv.FormatInt(int64(instanceId), 10)
				preStr := " line starter" + instanceIdstr + " "
				f.WriteString(preStr + logEntry[:(len(logEntry)-len(preStr))] + "\n")
			}
		}
		f.Sync()
		fileSize += (*eventsPerSecond) * (*eventSize)
		if fileSize >= logTruncateSize {
			os.Truncate(curFilePath, 0)
			f.Seek(0, 0)
			fileSize = 0
		}
		fmt.Printf("%s Ended.\n", time.Now().Format(layoutFormat))
	}
}
