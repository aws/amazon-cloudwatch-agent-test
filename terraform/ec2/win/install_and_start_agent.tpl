<powershell>
$installDirectory = "c:\temp\cw"
$downloadDirectory = $installDirectory
$logsDirectory = $installDirectory
$cwAgentInstaller = "$downloadDirectory\amazon-cloudwatch-agent.msi"
$cwAgentInstallPath = "C:\\Program Files\\Amazon\\AmazonCloudWatchAgent"

New-Item -ItemType "directory" -Path $installDirectory

Set-Location -Path $installDirectory

Write-host "Installing Powershell S3 CLI"
Install-PackageProvider NuGet -Force;
Set-PSRepository PSGallery -InstallationPolicy Trusted
Install-Module -Name AWS.Tools.S3 -AllowClobber

Write-host "Installing Cloudwatch Agent"
${copy_object}
Start-Process -FilePath msiexec -Args "/i $cwAgentInstaller /l*v $logsDirectory\installCWAgentLog.log /qn" -Verb RunAs -Wait

Write-host "Load config"

& "$cwAgentInstallPath\amazon-cloudwatch-agent-ctl.ps1" -a fetch-config -m ec2 -s -c ssm:${agent_json_config}
</powershell>