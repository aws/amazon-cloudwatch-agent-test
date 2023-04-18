package main

import (
	"flag"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path"
	"runtime"
	"strconv"
	"sync"
	"time"
)

const (
	layoutFormat    = "02 Jan 06 15:04:05 MST"
	logTruncateSize = 100 * 1024 * 1024
	multilineRatio  = 10 //means every 10 lines will have a multilineStarter(timestamp)
)

var (
	fileNum    = flag.Int("fileNum", 1, "Identify the file count.")
	eventRatio = flag.Int("eventRatio", 200, "Identify the log event count per second per file.")
	eventSize  = flag.Int("eventSize", 120, "Identify the single log line size, in Byte.")
	outPutPath = flag.String("path", "", "Identify the path where log files will generate.")
	filePrefix = flag.String("filePrefix", "tmp", "Identify the log file prefix")
	pprofPort  = flag.String("pprofPort", "", "pprof port to listen on")
	loopNum    = flag.Int64("loopNum", math.MaxInt64, "How many loops to write teh logentry.")
)

func main() {
	flag.Parse()
	fmt.Printf("Start writing %d files, and each file has throughput %d Bytes/sec...\n",
		*fileNum, (*eventRatio)*(*eventSize))
	if *pprofPort != "" {
		go func() {
			fmt.Printf("I! Starting pprof HTTP server at localhost:%s.\n", *pprofPort)
			if err := http.ListenAndServe("localhost:"+*pprofPort, nil); err != nil {
				fmt.Println("E! " + err.Error())
			}
		}()
	}
	if *outPutPath == "" {
		if "windows" == runtime.GOOS {
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
	var waitGroup = sync.WaitGroup{}
	//Start generating log
	for i := 0; i < *fileNum; i++ {
		waitGroup.Add(1)
		go writeLog(logEntry, i, *loopNum, &waitGroup)
	}
	waitGroup.Wait()
}

func writeLog(logEntry string, instanceId int, loopNum int64, waitGroup *sync.WaitGroup) {
	curFilePath := path.Join(*outPutPath, fmt.Sprintf("%s%d.log", *filePrefix, instanceId))
	fmt.Printf("Creating file %s\n", curFilePath)
	os.MkdirAll(*outPutPath, 0755)
	f, _ := os.Create(curFilePath)
	fileSize := 0
	// add jitter here to ensure multiple stream will not write at same time.
	r := time.Duration(rand.Intn(1000))
	time.Sleep(r * time.Millisecond)
	ticker := time.NewTicker(time.Second)
	var j int64
	for j = 0; j < loopNum; j++ {
		<-ticker.C
		fmt.Printf("%s Starting...\n", time.Now().Format(layoutFormat))
		for i := 0; i < *eventRatio; i++ {
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
		fileSize += (*eventRatio) * (*eventSize)
		if fileSize >= logTruncateSize {
			os.Truncate(curFilePath, 0)
			f.Seek(0, 0)
			fileSize = 0
		}
		fmt.Printf("%s Ended.\n", time.Now().Format(layoutFormat))
	}
	waitGroup.Done()
}
