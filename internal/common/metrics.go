// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"collectd.org/api"
	"collectd.org/exec"
	"collectd.org/network"
	"github.com/cactus/go-statsd-client/v5/statsd"
)

// StartSendingMetrics will generate metrics load based on the receiver (e.g 5000 statsd metrics per minute)
func StartSendingMetrics(receiver string, duration, sendingInterval time.Duration, metricPerInterval int) (err error) {
	go func() {
		switch receiver {
		case "statsd":
			err = sendStatsdMetrics(metricPerInterval, sendingInterval, duration)
		case "collectd":
			err = sendCollectDMetrics(metricPerInterval, sendingInterval, duration)
		default:
		}

	}()

	return err
}

func sendCollectDMetrics(metricPerMinute int, sendingInterval, duration time.Duration) error {
	// https://github.com/collectd/go-collectd/tree/92e86f95efac5eb62fa84acc6033e7a57218b606
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := network.Dial(
		net.JoinHostPort("127.0.0.1", network.DefaultService),
		network.ClientOptions{
			SecurityLevel: network.None,
		})

	if err != nil {
		return err
	}

	defer client.Close()

	ticker := time.NewTicker(sendingInterval)
	defer ticker.Stop()
	endTimeout := time.After(duration)

	for {
		select {
		case <-ticker.C:
			for t := 0; t < metricPerMinute/2; t++ {
				_ = client.Write(ctx, &api.ValueList{
					Identifier: api.Identifier{
						Host:   exec.Hostname(),
						Plugin: fmt.Sprint("gauge_", t),
						Type:   "gauge",
					},
					Time:     time.Now(),
					Interval: time.Minute,
					Values:   []api.Value{api.Gauge(t)},
				})

				err = client.Write(ctx, &api.ValueList{
					Identifier: api.Identifier{
						Host:   exec.Hostname(),
						Plugin: fmt.Sprint("counter_", t),
						Type:   "counter",
					},
					Time:     time.Now(),
					Interval: time.Minute,
					Values:   []api.Value{api.Counter(t)},
				})

				if err != nil && !errors.Is(err, network.ErrNotEnoughSpace) {
					return err
				}
			}

			if err := client.Flush(); err != nil {
				return err
			}
		case <-endTimeout:
			return nil
		}
	}

}

func sendStatsdMetrics(metricPerMinute int, sendingInterval, duration time.Duration) error {
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

	ticker := time.NewTicker(sendingInterval)
	defer ticker.Stop()
	endTimeout := time.After(duration)

	statsdDimension := []statsd.Tag{{"key", "val"}}
	for {
		select {
		case <-ticker.C:
			for t := 0; t < metricPerMinute/2; t++ {
				client.Inc(fmt.Sprint("counter_", t), int64(t), 1.0, statsdDimension...)
				client.Gauge(fmt.Sprint("gauge_", t), int64(t), 1, statsdDimension...)
			}
		case <-endTimeout:
			return nil
		}
	}
}
