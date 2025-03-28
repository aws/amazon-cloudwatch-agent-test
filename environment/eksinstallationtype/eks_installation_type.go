// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package eksinstallationtype

import "strings"

type EKSInstallationType string

const (
	HELM_CHART EKSInstallationType = "HELM_CHART"
	EKS_ADDON  EKSInstallationType = "EKS_ADDON"
)

var (
	eksInstallationTypes = map[string]EKSInstallationType{
		"HELM_CHART": HELM_CHART,
		"EKS_ADDON":  EKS_ADDON,
	}
)

func FromString(str string) (EKSInstallationType, bool) {
	t, ok := eksInstallationTypes[strings.ToUpper(str)]
	return t, ok
}
