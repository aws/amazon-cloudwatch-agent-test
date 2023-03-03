// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package common

import (
	"fmt"
	"sync"
	"time"

	"github.com/cactus/go-statsd-client/v5/statsd"
	"go.uber.org/multierr"
)

// StartLogWrite starts go routines to write logs to each of the logs that are monitored by CW Agent according to
// the config provided
func StartSendingMetrics(receiver string, agentRunDuration time.Duration, dataRate int) error {
	//create wait group so main test thread waits for log writing to finish before stopping agent and collecting data
	var (
		err      error
		multiErr error
		wg       sync.WaitGroup
	)

	wg.Add(1)
	go func() {
		defer wg.Done()
		switch receiver {
		case "statsd":
			err = sendStatsdMetrics(dataRate, agentRunDuration)

		default:
		}

		multiErr = multierr.Append(multiErr, err)
	}()

	wg.Wait()
	return multiErr
}

func sendStatsdMetrics(dataRate int, duration time.Duration) error {
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
			for t := 0; t < dataRate; t++ {
				client.Inc(fmt.Sprint(t), int64(t), 1.0)

			}
		case <-endTimeout:
			return nil
		}
	}

}
