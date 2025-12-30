# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: MIT

function Install-Java17 {
    param([string]$Bucket)

    $s3Key = "jdk17/windows/microsoft-jdk-17-windows-x64.zip"
    $localPath = "C:\tmp\jdk17.zip"
    
    Copy-S3Object -BucketName $Bucket -Key $s3Key -LocalFile $localPath
    Expand-Archive -Path $localPath -DestinationPath "C:\tmp\jdk17" -Force
}

function Setup-Jar {
    param(
        [string]$JarName = "test.jar",
        [switch]$WithCustomManifest
    )
    $ManifestArgs = $args
    
    $jdkPath = Get-ChildItem -Path "C:\tmp\jdk17" -Directory | Select-Object -First 1 -ExpandProperty FullName
    
    $tmpDir = "C:\tmp\jvm-compile"
    New-Item -Path $tmpDir -ItemType Directory -Force | Out-Null
    New-Item -Path "$tmpDir\META-INF" -ItemType Directory -Force | Out-Null
    
    $javaSource = @"
public class Main {
    public static void main(String[] args) {
        System.out.println("JVM Test Application Started");
        try {
            while (true) {
                Thread.sleep(5000);
                System.out.println("JVM Test Application Running...");
            }
        } catch (InterruptedException e) {
            System.out.println("JVM Test Application Interrupted");
            System.exit(0);
        }
    }
}
"@
    
    Set-Content -Path "$tmpDir\Main.java" -Value $javaSource
    & "$jdkPath\bin\javac.exe" "$tmpDir\Main.java"
    
    $manifestContent = "Manifest-Version: 1.0`n"

    foreach ($arg in $ManifestArgs) {
        if ($arg -match "=") {
            $key, $value = $arg -split "=", 2
            $manifestContent += "$key`: $value`n"
        }
    }

    $manifestContent += "`n"

    [System.IO.File]::WriteAllText("$tmpDir\META-INF\MANIFEST.MF", $manifestContent, [System.Text.Encoding]::ASCII)
    
    $originalLocation = Get-Location
    try {
        Set-Location $tmpDir
        & "$jdkPath\bin\jar.exe" cfm "C:\tmp\$JarName" "META-INF\MANIFEST.MF" "*.class"
    } finally {
        Set-Location $originalLocation
        Remove-Item -Path $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

function Setup-Tomcat {
    param(
        [string]$Version = "apache-tomcat-9.0.110",
        [string]$Bucket
    )
    
    $isWin2016 = (Get-WmiObject -Class Win32_OperatingSystem).Caption -match "2016"
    
    Copy-S3Object -BucketName $Bucket -Key "tomcat/$Version.tar.gz" -LocalFile "C:\tmp\tomcat\$Version.tar.gz"
    
    if ($isWin2016) {
        $originalLocation = Get-Location
        try {
            Set-Location "C:\tmp\tomcat"
            & "C:\Program Files\Git\usr\bin\gzip.exe" -d "$Version.tar.gz"
            & "C:\Program Files\Git\usr\bin\tar.exe" -xf "$Version.tar"
        } finally {
            Set-Location $originalLocation
        }
    } else {
        tar -xzf "C:\tmp\tomcat\$Version.tar.gz" -C "C:\tmp\tomcat"
    }
}

function Setup-Kafka {
    param(
        [string]$Version = "kafka_2.13-3.5.0",
        [string]$Bucket
    )
    
    $isWin2016 = (Get-WmiObject -Class Win32_OperatingSystem).Caption -match "2016"
    
    Copy-S3Object -BucketName $Bucket -Key "kafka/$Version.tgz" -LocalFile "C:\tmp\kafka\$Version.tgz"
    
    if ($isWin2016) {
        $originalLocation = Get-Location
        try {
            Set-Location "C:\tmp\kafka"
            & "C:\Program Files\Git\usr\bin\gzip.exe" -d "$Version.tgz"
            & "C:\Program Files\Git\usr\bin\tar.exe" -xf "$Version.tar"
        } finally {
            Set-Location $originalLocation
        }
    } else {
        tar -xzf "C:\tmp\kafka\$Version.tgz" -C "C:\tmp\kafka"
    }
    
    $kafkaDir = "C:\tmp\kafka\$Version"
}

function Start-JVM {
    param(
        [string]$JarPath = "C:\tmp\test.jar",
        [int]$Port
    )

    $jdkPath = Get-ChildItem -Path "C:\tmp\jdk17" -Directory | Select-Object -First 1 -ExpandProperty FullName
    
    if ($Port) {
        $process = Start-Process -FilePath "$jdkPath\bin\java.exe" -ArgumentList @(
            "-Dcom.sun.management.jmxremote",
            "-Dcom.sun.management.jmxremote.port=$Port",
            "-Dcom.sun.management.jmxremote.local.only=false",
            "-Dcom.sun.management.jmxremote.authenticate=false",
            "-Dcom.sun.management.jmxremote.ssl=false",
            "-Dcom.sun.management.jmxremote.rmi.port=$Port",
            "-Dcom.sun.management.jmxremote.host=localhost",
            "-Djava.rmi.server.hostname=localhost",
            "-jar", $JarPath
        ) -PassThru
    } else {
        $process = Start-Process -FilePath "$jdkPath\bin\java.exe" -ArgumentList @("-jar", $JarPath) -PassThru
    }
    
    return $process.Id
}

function Stop-JVM {
    param([int]$ProcessId)
    
    Stop-Process -Id $ProcessId -Force -ErrorAction SilentlyContinue
    while (Get-Process -Id $ProcessId -ErrorAction SilentlyContinue) {
        Start-Sleep -Milliseconds 100
    }
}

function Start-Tomcat {
    param(
        [string]$TomcatDir,
        [int]$Port
    )
    
    $jdkPath = Get-ChildItem -Path "C:\tmp\jdk17" -Directory | Select-Object -First 1 -ExpandProperty FullName
    $TomcatDir = $TomcatDir.TrimEnd('\')
    $env:JAVA_HOME = $jdkPath
    $env:CATALINA_HOME = $TomcatDir
    $env:CATALINA_BASE = $TomcatDir

    if ($Port) {
        $env:CATALINA_OPTS = "-Dcom.sun.management.jmxremote.port=$Port -Dcom.sun.management.jmxremote.authenticate=false -Dcom.sun.management.jmxremote.ssl=false"
    }
    
    Start-Process -FilePath "$TomcatDir\bin\startup.bat" -WindowStyle Hidden -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 15
    
    $tomcatProcess = Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.ProcessName -match "java" -and $_.CommandLine -match $TomcatDir } | Select-Object -First 1
    if ($tomcatProcess) { return $tomcatProcess.Id } else { return 0 }
}

function Stop-Tomcat {
    param(
        [int]$ProcessId,
        [string]$TomcatDir
    )
    
    $jdkPath = Get-ChildItem -Path "C:\tmp\jdk17" -Directory | Select-Object -First 1 -ExpandProperty FullName
    $TomcatDir = $TomcatDir.TrimEnd('\')
    $env:JAVA_HOME = $jdkPath
    $env:CATALINA_HOME = $TomcatDir
    $env:CATALINA_BASE = $TomcatDir

    & "$TomcatDir\bin\shutdown.bat" | Out-Null
    
    if ($ProcessId -eq 0) {
        $tomcatProcess = Get-Process | Where-Object { $_.ProcessName -match "java" -and $_.CommandLine -match $TomcatDir } | Select-Object -First 1
        if ($tomcatProcess) { $ProcessId = $tomcatProcess.Id } else { $ProcessId = 0 }
    }
    
    if ($ProcessId -ne 0) {
        Stop-Process -Id $ProcessId -Force -ErrorAction SilentlyContinue
        while (Get-Process -Id $ProcessId -ErrorAction SilentlyContinue) {
            Start-Sleep -Milliseconds 100
        }
    } else {
        Get-Process -Name java -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue # If still can't find the Tomcat process, then reset Java processes
    }
    
    Remove-Item -Path "env:CATALINA_OPTS", "env:CATALINA_BASE", "env:CATALINA_HOME" -ErrorAction SilentlyContinue
}

function Start-Kafka {
    param(
        [string]$KafkaDir,
        [string]$Version,
        [int]$Port
    )

    $jdkPath = Get-ChildItem -Path "C:\tmp\jdk17" -Directory | Select-Object -First 1 -ExpandProperty FullName
    $env:JAVA_HOME = $jdkPath
    $env:KAFKA_HEAP_OPTS = "-Xmx256M -Xms256M"
    $env:PATH = "$jdkPath\bin;$env:PATH"

    Remove-Item -Path "C:\tmp\zookeeper", "C:\tmp\kafka-logs", "C:\tmp\kraft-controller-logs" -Recurse -Force -ErrorAction SilentlyContinue
    
    $majorVersion = ($Version -split '-')[1] -split '\.' | Select-Object -First 1
    
    if ([int]$majorVersion -lt 4) {
        Start-Process -FilePath "$KafkaDir\bin\windows\zookeeper-server-start.bat" -ArgumentList "$KafkaDir\config\zookeeper.properties" -WindowStyle Hidden
        $configFile = "$KafkaDir\config\server.properties"
    } else {
        $configFile = "$KafkaDir\config\controller.properties"
        $uuid = (& "$KafkaDir\bin\windows\kafka-storage.bat" random-uuid) | Select-Object -Last 1
        $uuid = $uuid.Trim()
        & "$KafkaDir\bin\windows\kafka-storage.bat" format --cluster-id $uuid --config $configFile --standalone | Out-Null
    }
    
    if ($Port) {
        $env:KAFKA_JMX_OPTS = "-Dcom.sun.management.jmxremote.port=$Port -Dcom.sun.management.jmxremote.authenticate=false -Dcom.sun.management.jmxremote.ssl=false"
    }
    
    $kafkaProcess = Start-Process -FilePath "$KafkaDir\bin\windows\kafka-server-start.bat" -ArgumentList $configFile -WindowStyle Hidden -PassThru
    Start-Sleep -Seconds 5
    
    return $kafkaProcess.Id
}

function Stop-Kafka {
    param(
        [int]$ProcessId,
        [string]$KafkaDir,
        [string]$Version
    )

    $jdkPath = Get-ChildItem -Path "C:\tmp\jdk17" -Directory | Select-Object -First 1 -ExpandProperty FullName
    $env:JAVA_HOME = $jdkPath
    
    & "$KafkaDir\bin\windows\kafka-server-stop.bat" | Out-Null
    
    $majorVersion = ($Version -split '-')[1] -split '\.' | Select-Object -First 1
    if ([int]$majorVersion -lt 4) {
        & "$KafkaDir\bin\windows\zookeeper-server-stop.bat" | Out-Null
    }
    
    if ($ProcessId) {
        Stop-Process -Id $ProcessId -Force -ErrorAction SilentlyContinue
        while (Get-Process -Id $ProcessId -ErrorAction SilentlyContinue) {
            Start-Sleep -Milliseconds 100
        }
    }
    
    Remove-Item -Path "env:KAFKA_JMX_OPTS", "env:KAFKA_HEAP_OPTS" -ErrorAction SilentlyContinue
}

function Install-NvidiaDriver {
    param([string]$Bucket)
    
    $isWin2016or2019 = (Get-WmiObject -Class Win32_OperatingSystem).Caption -match "2016|2019"
    
    if ($isWin2016or2019) {
        $s3Key = "nvidia/windows/grid-9.1/431.79_grid_win10_server2016_server2019_64bit_international.exe"
    } else {
        $s3Key = "nvidia/windows/latest/581.42_grid_win10_win11_server2019_server2022_server2025_dch_64bit_international_aws_swl.exe"
    }
    
    $localPath = "C:\tmp\nvidia-driver.exe"
    Copy-S3Object -BucketName $Bucket -Key $s3Key -LocalFile $localPath
    
    Start-Process -FilePath $localPath -ArgumentList "/s", "/noreboot" -Wait -PassThru
}

function Uninstall-NvidiaDriver {
    Remove-Item -Path "C:\Windows\System32\nvidia-smi.exe" -Force -ErrorAction SilentlyContinue
    Remove-Item -Path "C:\Program Files\NVIDIA Corporation\NVSMI\nvidia-smi.exe" -Force -ErrorAction SilentlyContinue
    Remove-Item -Path "C:\tmp\nvidia-driver.exe" -Force -ErrorAction SilentlyContinue
}

if ($args.Count -gt 0) {
    $FunctionName = $args[0]
    $FunctionArgs = $args[1..($args.Count-1)]
    & $FunctionName @FunctionArgs
}