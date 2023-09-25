package emf_concurrent

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"
)

const (
	metadataName  = "_aws"
	namespace     = "ConcurrentEMFTest"
	metricName1   = "ExecutionTime"
	metricName2   = "DuplicateExecutionTime"
	metricValue   = 1.23456789
	metricUnit    = "Seconds"
	dimensionName = "Dimension"
	randomName    = "Random"
	letters       = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
)

var (
	newLineChar = []byte("\n")
)

type Metadata struct {
	Timestamp         int64      `json:"Timestamp"`
	LogGroupName      string     `json:"LogGroupName"`
	LogStreamName     string     `json:"LogStreamName"`
	CloudWatchMetrics []CWMetric `json:"CloudWatchMetrics"`
}

type CWMetric struct {
	Namespace  string     `json:"Namespace"`
	Dimensions [][]string `json:"Dimensions"`
	Metrics    []Metric   `json:"Metrics"`
}

type Metric struct {
	Name string `json:"Name"`
	Unit string `json:"Unit"`
}

type emitter struct {
	wg            sync.WaitGroup
	done          chan struct{}
	interval      time.Duration
	logGroupName  string
	logStreamName string
	dimension     string
}

func (e *emitter) start(conn *net.TCPConn) {
	defer e.wg.Done()
	ticker := time.NewTicker(e.interval)
	metadata := e.createMetadata()
	for {
		select {
		case <-e.done:
			ticker.Stop()
			return
		case <-ticker.C:
			metadata.Timestamp = time.Now().UnixMilli()
			_, _ = conn.Write(e.createEmfLog(metadata))
		}
	}
}

func (e *emitter) createMetadata() *Metadata {
	return &Metadata{
		Timestamp:     time.Now().UnixMilli(),
		LogGroupName:  e.logGroupName,
		LogStreamName: e.logStreamName,
		CloudWatchMetrics: []CWMetric{
			{
				Namespace:  namespace,
				Dimensions: [][]string{{dimensionName}},
				Metrics: []Metric{
					{Name: metricName1, Unit: metricUnit},
					{Name: metricName2, Unit: metricUnit},
				},
			},
		},
	}
}

func (e *emitter) createEmfLog(metadata *Metadata) []byte {
	r := rand.Intn(99) + 1
	emfLog := map[string]interface{}{
		metadataName:  metadata,
		dimensionName: e.dimension,
		metricName1:   metricValue,
		metricName2:   metricValue,
		// introduces variability in payload size
		randomName: fmt.Sprintf("https://www.amazon.com/%s", randString(r)),
	}
	content, _ := json.Marshal(emfLog)
	return append(content, newLineChar...)
}

func randString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
