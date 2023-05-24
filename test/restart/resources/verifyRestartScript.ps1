Function countLogLines() {
    Param (
        [Parameter(Mandatory = $true)]
        [string]$log_path
    )
    if (Test-Path -LiteralPath "$log_path") {
        return (Get-Content $log_path).Length
    } else {
        return "0"
    }
}
$cwa_log=countLoglines("$Env:ProgramData\Amazon\AmazonCloudWatchAgent\Logs\amazon-cloudwatch-agent.log")
Write-Output "cwa_log:$cwa_log"