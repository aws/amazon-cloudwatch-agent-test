package main

import (
	"flag"
	"log"
	"math/rand"
	"strconv"
	"time"

	"github.com/DataDog/datadog-go/statsd"
)

const operationNum = 5 // the number of operations done in one loop

var (
	clientNum = flag.Int("clientNum", 1, "The number of statsd client.")
	tps = flag.Int("tps", 100, "Transaction per second for each statsd client.")
	metricNum = flag.Int("metricNum", 100, "The number of unique metrics for each statsd client.")
)

//	sample command:
//		statsdGen -clientNum 1 -tps 100 -metricNum 100
func main() {
	flag.Parse()
	log.Printf("Start statsd generator %d client, and each client sends %d tps with %d unique metrics...", *clientNum, *tps, *metricNum)

	for i := 0; i < *clientNum; i++ {
		go startStatsDClient(i, *tps, *metricNum)
	}
	//wait forever
	select {}
}

func startStatsDClient(clientId, tps, metricNum int) {
	// Create the client
	c, err := statsd.NewBuffered("127.0.0.1:8125", 100)
	if err != nil {
		log.Fatal(err)
	}
	// Prefix every metric with the app name
	c.Namespace = "SoakTest."
	c.Tags = append(c.Tags, "clientId:"+strconv.Itoa(clientId), "region:us-west-2", "airportCode:pdx", "tag_name_only",
		"long_tag_name.long_tag_name.long_tag_name.long_tag_name.long_tag_name:long_tag_value.long_tag_value.long_tag_value.long_tag_value.long_tag_value")

	loopNum := metricNum / operationNum

	//use float64 to avoid dividing by 0.
	sendRate := 1000 / (float64(tps) / float64(metricNum))
	ticker := time.NewTicker(time.Millisecond * time.Duration(sendRate))

	for {
		select {
		case <-ticker.C:
			current := time.Now()
			for i := 0; i < loopNum; i++ {
				iString := strconv.Itoa(i)
				if err = c.Gauge("request.Gauge."+iString, 12, []string{"type:Gauge"}, 1); err != nil {
					log.Printf("Client %v Func %v err: %v", clientId, "Gauge", err)
				}
				if err = c.Timing("request.Timing."+iString, time.Millisecond*time.Duration(rand.Float64()*100), []string{"type:Timing"}, 1); err != nil {
					log.Printf("Client %v Func %v err: %v", clientId, "Timing", err)
				}
				if err = c.Count("request.Count."+iString, 2, []string{"type:Count"}, 1); err != nil {
					log.Printf("Client %v Func %v err: %v", clientId, "Count", err)
				}
				if err = c.Set("request.Set."+iString, strconv.Itoa(rand.Intn(1000)), []string{"type:Set"}, 1); err != nil {
					log.Printf("Client %v Func %v err: %v", clientId, "Set", err)
				}
				if err = c.Histogram("request.Histogram."+iString, rand.Float64()*1000, []string{"type:Histogram"}, 1); err != nil {
					log.Printf("Client %v Func %v err: %v", clientId, "Histogram", err)
				}
			}
			if timeCost := time.Now().Sub(current); timeCost > time.Duration(sendRate)*time.Millisecond {
				log.Printf("Completed %v request in %v, supposed to be completed within %v milliseconds.", operationNum*loopNum, timeCost, sendRate)
			}
		}
	}
}
