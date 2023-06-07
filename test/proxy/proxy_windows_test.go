// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows
// +build windows

package proxy

func GetCommandToCreateProxyConfig(proxyUrl string) []string {
	return []string{
		fmt.Sprintf("echo '\n[proxy]\n  http_proxy = \\\"%s\\\"\n  no_proxy = \\\"169.254.169.254\\\"' | Set-Content -Path \"%s\"", t.proxyUrl, "${Env:ProgramData}\\Amazon\\AmazonCloudWatchAgent\\common-config.toml"),
	}
}

func getDimensions(t *RoleTestRunner, instanceId string) {
	return []types.Dimension{
		types.Dimension{Name: "InstanceId", Value: instanceId},
		types.Dimension{Name: "cpu", Value: aws.String("cpu-total")},
	}
}
