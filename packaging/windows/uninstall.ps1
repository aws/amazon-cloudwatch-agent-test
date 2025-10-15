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

$AmazonProgramFiles = "${Env:ProgramFiles}\Amazon"
$CWAProgramFiles = "${AmazonProgramFiles}\AmazonCloudWatchAgent"
$Cmd = "${CWAProgramFiles}\amazon-cloudwatch-agent-ctl.ps1"

if (Test-Path -LiteralPath "${Cmd}" -PathType Leaf) {
    & "${Cmd}" -Action prep-restart
    & "${Cmd}" -Action preun
}

function debug($msg) {Write-Host "$(Get-Date -Format o): [DEBUG] $msg"}
function info($msg) {Write-Host "$(Get-Date -Format o): [INFO] $msg"}
function warn($msg) {Write-Host "$(Get-Date -Format o): [WARN] $msg"}
function error($msg) {Write-Host "$(Get-Date -Format o): [ERROR] $msg"}

function Get-InstalledAgent() {
    [System.Management.ManagementObject[]] $agents = Get-WmiObject -Class Win32_Product -ComputerName . -Filter "Name='Amazon CloudWatch Agent'"
    debug "Installed Amazon CloudWatch agents:"
    debug $agents

    if (!$agents -or $agents.Count -eq 0) {
        info "No existing agent installed."
        return $null
    }

    return $agents
}


info "Starting update check..."

$installed_agents = Get-InstalledAgent

# uninstall the old agent
if($installed_agents) {
    for ($i = 0; $i -lt @($installed_agents).Count; $i ++) {
        info "Uninstalling existing agent..."
        info @($installed_agents)[$i]
        $uninstall = @($installed_agents)[$i].Uninstall()
        if ($uninstall.ReturnValue -ne 0) {
            warn "Uninstalling existing agent failed."
            Exit 1
        }
    }

}

info "Uninstall complete."



