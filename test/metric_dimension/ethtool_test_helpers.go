// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package metric_dimension

import "net"

// getNetworkInterface returns the first available network interface
// from a list of common interface names used in EC2.
// Returns empty string if no suitable interface is found.
func getNetworkInterface() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		// eth0: traditional Linux naming (common on older EC2 instances)
		// ens5: predictable network interface naming (common on newer EC2 instances)
		if iface.Name == "eth0" || iface.Name == "ens5" {
			return iface.Name
		}
	}
	return ""
}
