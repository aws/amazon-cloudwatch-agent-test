// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package common

import (
	"fmt"
	"time"

	"github.com/cactus/go-statsd-client/v5/statsd"
	"go.uber.org/multierr"
)

// StartSendingMetrics will generate metrics load based on the receiver (e.g 5000 statsd metrics per minute)
func StartSendingMetrics(receiver string, duration time.Duration, metricPerMinute int) error {
	var (
		err      error
		multiErr error
	)

	go func() {
		switch receiver {
		case "statsd":
			err = sendStatsdMetrics(metricPerMinute, duration)
		default:
		}

		multiErr = multierr.Append(multiErr, err)
	}()

	return multiErr
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
			for t := 0; t < metricPerMinute; t++ {
				client.Inc(fmt.Sprint(t), int64(t), 1.0)
			}
		case <-endTimeout:
			return nil
		}
	}
}
