# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: MIT

# SQL Server DBI Integration Test Setup for Windows
# Configures SQL Server Express for Database Insights testing:
# - Enables TCP/IP and mixed-mode authentication
# - Sets sa password and enables the account
# - Creates testdb database and test_orders table
# - Enables Query Store for top SQL metrics
# - Grants necessary permissions for DBI monitoring

$ErrorActionPreference = "Stop"

$saPassword = "IntegTest#P@ss1"
$serverInstance = "localhost"

Write-Host "=== [1/6] Enabling TCP/IP and SQL Server Authentication ==="
# Import SQL Server module (available on Windows Server with SQL Server)
Import-Module SqlServer -ErrorAction SilentlyContinue

# Enable TCP/IP protocol using WMI (works without SQLPS)
$wmi = New-Object Microsoft.SqlServer.Management.Smo.Wmi.ManagedComputer
$tcp = $wmi.ServerInstances['MSSQLSERVER'].ServerProtocols['Tcp']
$tcp.IsEnabled = $true
$tcp.Alter()

# Enable mixed-mode authentication (requires registry edit + restart)
$regPath = "HKLM:\SOFTWARE\Microsoft\Microsoft SQL Server\MSSQL16.MSSQLSERVER\MSSQLServer"
Set-ItemProperty -Path $regPath -Name "LoginMode" -Value 2

Write-Host "=== [2/6] Restarting SQL Server to apply protocol changes ==="
Restart-Service MSSQLSERVER -Force
Start-Sleep -Seconds 10

Write-Host "=== [3/6] Configuring sa account ==="
# Use sqlcmd to enable sa and set password (works with Windows auth before sa is enabled)
$sqlcmd = "C:\Program Files\Microsoft SQL Server\Client SDK\ODBC\170\Tools\Binn\SQLCMD.EXE"
& $sqlcmd -S $serverInstance -E -Q "ALTER LOGIN sa WITH PASSWORD='$saPassword'; ALTER LOGIN sa ENABLE;"

Write-Host "=== [4/6] Creating testdb database ==="
& $sqlcmd -S $serverInstance -U sa -P $saPassword -C -N -Q @"
IF NOT EXISTS (SELECT name FROM sys.databases WHERE name = 'testdb')
CREATE DATABASE testdb;
"@

Write-Host "=== [5/6] Creating test schema and enabling Query Store ==="
& $sqlcmd -S $serverInstance -U sa -P $saPassword -C -N -d testdb -Q @"
-- Create test table for workload
IF OBJECT_ID('test_orders', 'U') IS NULL
CREATE TABLE test_orders (
    order_id INT IDENTITY(1,1) PRIMARY KEY,
    customer_name NVARCHAR(100),
    amount DECIMAL(10,2),
    created_at DATETIME2 DEFAULT SYSUTCDATETIME()
);

-- Enable Query Store (required for Top SQL metrics from query_store DMVs)
ALTER DATABASE testdb SET QUERY_STORE = ON;
ALTER DATABASE testdb SET QUERY_STORE (
    OPERATION_MODE = READ_WRITE,
    DATA_FLUSH_INTERVAL_SECONDS = 60,
    INTERVAL_LENGTH_MINUTES = 1,
    MAX_STORAGE_SIZE_MB = 100,
    QUERY_CAPTURE_MODE = ALL
);
"@

Write-Host "=== [6/6] Creating credentials file ==="
# Connection string format read by the SQL Server receiver
$credPath = "C:\ProgramData\Amazon\AmazonCloudWatchAgent\.sqlserver_credentials"
$credDir = Split-Path $credPath -Parent
if (-not (Test-Path $credDir)) {
    New-Item -ItemType Directory -Path $credDir -Force | Out-Null
}
"server=localhost;port=1433;user id=sa;password=$saPassword" | Set-Content -Path $credPath -NoNewline

Write-Host "=== [7/7] Verifying setup ==="
& $sqlcmd -S $serverInstance -U sa -P $saPassword -C -N -Q @"
SELECT @@VERSION AS SQLServerVersion;
SELECT name, state_desc FROM sys.databases WHERE name = 'testdb';
"@

Write-Host "=== SQL Server setup complete ==="
