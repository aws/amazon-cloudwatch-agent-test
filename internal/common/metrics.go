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
func StartSendingMetrics(receivers []string, agentRunDuration time.Duration, dataRate int) error {
	//create wait group so main test thread waits for log writing to finish before stopping agent and collecting data
	var (
		wg       sync.WaitGroup
		multiErr error
	)

	for _, receiver := range receivers {
		wg.Add(1)
		go func() {
			var err error
			defer wg.Done()
			switch receiver {
			case "statsd":
				err = sendStatsdMetrics(dataRate)

			default:
			}

			multiErr = multierr.Append(multiErr, err)

		}()
	}

	//wait until writing to logs finishes
	wg.Wait()
	return multiErr
}

func sendStatsdMetrics(dataRate int) error {
	client, err := statsd.NewClient("127.0.0.1:8125", "test-client")

	if err != nil {
		return err
	}

	defer client.Close()

	for time := 0; time < dataRate; time++ {
		go func() {
			client.Inc(fmt.Sprintf("statsd_%v", time), int64(time), 1.0)
		}()
	}
	return nil
}
