# Copyright 2017 Amazon.com, Inc. and its affiliates. All Rights Reserved.
#
# Licensed under the Amazon Software License (the "License").
# You may not use this file except in compliance with the License.
# A copy of the License is located at
#
#   http://aws.amazon.com/asl/
#
# or in the "license" file accompanying this file. This file is distributed
# on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
# express or implied. See the License for the specific language governing
# permissions and limitations under the License.

Set-StrictMode -Version 2.0
$ErrorActionPreference = "Stop"

Start-Process msiexec.exe -Wait -ArgumentList '/i amazon-cloudwatch-agent.msi'

$CWADirectory = 'Amazon\AmazonCloudWatchAgent'
$CWAProgramFiles = "${Env:ProgramFiles}\${CWADirectory}"
$Cmd = "${CWAProgramFiles}\amazon-cloudwatch-agent-ctl.ps1"

& "${Cmd}" -Action cond-restart
