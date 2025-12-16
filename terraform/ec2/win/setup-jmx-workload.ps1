# JMX Workload Setup Script
# This script sets up JMX monitoring with Prometheus for CloudWatch Agent testing

Write-Output "Starting JMX workload setup..."

# Create working directory
New-Item -ItemType Directory -Path "C:\jmx_workload" -Force
Write-Output "Created JMX workload directory"

# Create JMX exporter config file
@"
---
rules:
- pattern: ".*"
"@ | Set-Content -Path "C:\jmx_workload\exporter_config.yaml"
Write-Output "Created JMX exporter config file"

# Create Prometheus config file
@"
global:
  scrape_interval: 1m
  scrape_timeout: 10s
scrape_configs:
  - job_name: jmx-exporter
    sample_limit: 10000
    file_sd_configs:
      - files: [ "C:\\jmx_workload\\prometheus_file_sd.yaml" ]
"@ | Set-Content -Path "C:\jmx_workload\prometheus.yaml"
Write-Output "Created Prometheus config file"

# Get instance metadata and create Prometheus service discovery file
Write-Output "Retrieving instance metadata..."
$env:AWS_IMDSV2_TOKEN = (Invoke-RestMethod -Uri 'http://169.254.169.254/latest/api/token' -Method 'PUT' -Headers @{ 'X-aws-ec2-metadata-token-ttl-seconds' = '300' }).trim()
$InstanceId = Invoke-RestMethod -Uri 'http://169.254.169.254/latest/meta-data/instance-id' -Headers @{ 'X-aws-ec2-metadata-token' = $env:AWS_IMDSV2_TOKEN }

@"
- targets:
  - 127.0.0.1:9404
  labels:
    application: test-app
    InstanceId: $InstanceId
"@ | Set-Content -Path "C:\jmx_workload\prometheus_file_sd.yaml"
Write-Output "Created Prometheus service discovery file with Instance ID: $InstanceId"

# Download JMX Prometheus agent and sample Java application
Write-Output "Downloading JMX Prometheus agent and sample application..."
[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.SecurityProtocolType]::Tls12

try {
    (New-Object Net.WebClient).DownloadFile(
        'https://cwagent-prometheus-test.s3-us-west-2.amazonaws.com/jmx_prometheus_javaagent-0.12.0.jar',
        'C:\jmx_workload\jmx_prometheus_javaagent-0.12.0.jar'
    )
    Write-Output "Downloaded JMX Prometheus agent"
    
    (New-Object Net.WebClient).DownloadFile(
        'https://cwagent-prometheus-test.s3-us-west-2.amazonaws.com/SampleJavaApplication-1.0-SNAPSHOT.jar',
        'C:\jmx_workload\SampleJavaApplication-1.0-SNAPSHOT.jar'
    )
    Write-Output "Downloaded sample Java application"
} catch {
    Write-Error "Failed to download files: $_"
    exit 1
}

# Wait for system to stabilize
Write-Output "Waiting 60 seconds for system stabilization..."
Start-Sleep -s 60

# Find Java installation and start the sample application with JMX agent
Write-Output "Finding Java installation..."
$javaPath = (Get-ChildItem 'C:\Program Files\OpenJDK' -Directory | 
    Sort-Object Name -Descending | 
    Select-Object -First 1 | 
    ForEach-Object { Join-Path $_.FullName 'bin\java.exe' })

if (-not $javaPath -or -not (Test-Path $javaPath)) {
    Write-Error "Java installation not found"
    exit 1
}

Write-Output "Using Java at: $javaPath"

# Start the Java application with JMX agent
$javaArgs = @(
    "-javaagent:C:\jmx_workload\jmx_prometheus_javaagent-0.12.0.jar=9404:C:\jmx_workload\exporter_config.yaml",
    "-cp", "C:\jmx_workload\SampleJavaApplication-1.0-SNAPSHOT.jar",
    "com.gubupt.sample.app.App"
)

try {
    Start-Process -FilePath $javaPath -ArgumentList $javaArgs -WindowStyle Hidden
    Write-Output "Started Java application with JMX agent"
} catch {
    Write-Error "Failed to start Java application: $_"
    exit 1
}

# Wait for application to start
Write-Output "Waiting 30 seconds for application startup..."
Start-Sleep -s 30

# Verify JMX metrics endpoint is working with retry logic
Write-Output "Verifying JMX metrics endpoint..."
$retries = 0
$maxRetries = 5

while ($retries -lt $maxRetries) {
    try {
        $response = Invoke-WebRequest -Uri "http://localhost:9404/metrics" -UseBasicParsing -TimeoutSec 5
        
        if ($response.StatusCode -eq 200 -and $response.Content -match 'jvm_threads_current') {
            Write-Output "JMX metrics endpoint is working and jvm_threads_current is available"
            break
        }
    } catch {
        $retries++
        Write-Output "Attempt $retries/$maxRetries: JMX endpoint not ready, retrying..."
        
        if ($retries -lt $maxRetries) {
            Start-Sleep -s 10
        }
    }
}

if ($retries -eq $maxRetries) {
    Write-Error "JMX metrics endpoint failed to start properly after $maxRetries attempts"
    exit 1
}

Write-Output "JMX workload setup completed successfully!"