// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package performance

type PerformanceInformation map[string]interface{}

/*
	Contains the following:
		// A service name we want to monitor (e.g CloudWatchAgent)
		"Service":          ServiceName,
		// A use case for generate metrics loads (e.g statsd, collectd)
		"UseCase":          receiver,
		// Commit Information
		"CommitDate":       commitDate,
		"CommitHash":       commitHash,
		// Data type (e.g metrics/traces/logs)
		"DataType":         dataType,
		// Performance metrics of the monitored services.
		"Results":          result,
		// The duration of time when running the services
		"CollectionPeriod": collectionPeriod,
		"InstanceAMI":      instanceAMI,
		"InstanceType":     instanceType,
*/
