// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package performance

type PerformanceInformation map[string]interface{}

/*
	Contains the following:
		"Service":          ServiceName,
		"UniqueID":         uniqueID,
		"UseCase":          receiver,
		"CommitDate":       commitDate,
		"CommitHash":       commitHash,
		"DataType":         dataType,
		"Results":          result,
		"CollectionPeriod": collectionPeriod,
		"InstanceAMI":      instanceAMI,
		"InstanceType":     instanceType,
*/
