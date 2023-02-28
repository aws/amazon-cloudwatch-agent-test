// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package common

import (
	"fmt"
	"time"

	"github.com/cactus/go-statsd-client/v5/statsd"
)

// StartLogWrite starts go routines to write logs to each of the logs that are monitored by CW Agent according to
// the config provided
func StartSendingMetrics(receiver string, agentRunDuration time.Duration, metricPerMinute int) error {
	var err error
	switch receiver {
	case "statsd":
		err = sendStatsdMetrics(metricPerMinute, agentRunDuration)

	default:
	}
	return err
}

func sendStatsdMetrics(metricPerMinute int, duration time.Duration) error {
	// https://github.com/cactus/go-statsd-client#example
	statsdClientConfig := &statsd.ClientConfig{
		Address:     ":8125",
		Prefix:      "statsd",
		UseBuffered: true,
		// interval to force flush buffer. full buffers will flush on their own,
		// but for data not frequently sent, a max threshold is useful
		FlushInterval: 300 * time.Millisecond,
	}
	client, err := statsd.NewClientWithConfig(statsdClientConfig)
	if err != nil {
		return err
	}

	defer client.Close()

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	endTimeout := time.After(duration)

	for {
		select {
		case <-ticker.C:
			for time := 0; time < metricPerMinute; time++ {
				go func(time int) {
					client.Inc(fmt.Sprintf("statsd_metric_%v", time), int64(time), 1.0)
				}(time)
			}
		case <-endTimeout:
			return nil
		}
	}

}
