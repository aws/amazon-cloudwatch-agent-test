# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: MIT

function Install-Java17 {
    param([string]$Bucket)
    
    # main java installation
    $s3Key = "jdk17/windows/microsoft-jdk-17-windows-x64.zip"
    $localPath = "C:\tmp\jdk17.zip"
    
    Copy-S3Object -BucketName $Bucket -Key $s3Key -LocalFile $localPath
    Expand-Archive -Path $localPath -DestinationPath "C:\tmp\jdk17" -Force

    # override existing java installation from ami (TODO: remove when ami uses jdk17 by default)
    & "C:\ProgramData\chocolatey\bin\choco.exe" install openjdk --version=17.0.2 --confirm --force
    
    $currentPath = [Environment]::GetEnvironmentVariable("PATH", "Machine")
    $newPath = $currentPath -replace "C:\\Program Files\\OpenJDK\\jdk-15\.[^;]*\\bin;?", ""
    $newPath = "C:\Program Files\OpenJDK\jdk-17.0.2\bin;$newPath"
    [Environment]::SetEnvironmentVariable("PATH", $newPath, "Machine")
    
    $env:JAVA_HOME = "C:\Program Files\OpenJDK\jdk-17.0.2"
    $env:PATH = "$env:JAVA_HOME\bin;$env:PATH"
    [Environment]::SetEnvironmentVariable("JAVA_HOME", "C:\Program Files\OpenJDK\jdk-17.0.2", "Machine")
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
        Start-Sleep -Seconds 1
        Remove-Item -Path $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

function Setup-Tomcat {
    param(
        [string]$Version = "apache-tomcat-9.0.110",
        [string]$Bucket
    )
    
    $isWin2016 = (Get-WmiObject -Class Win32_OperatingSystem).Caption -match "2016"
    
    Copy-S3Object -BucketName $Bucket -Key "tomcat/$Version.tar.gz" -LocalFile "C:\tmp\$Version.tar.gz"
    
    if ($isWin2016) {
        $originalLocation = Get-Location
        try {
            Set-Location "C:\tmp"
            & "C:\Program Files\Git\usr\bin\gzip.exe" -d "$Version.tar.gz"
            & "C:\Program Files\Git\usr\bin\tar.exe" -xf "$Version.tar"
        } finally {
            Set-Location $originalLocation
        }
    } else {
        tar -xzf "C:\tmp\$Version.tar.gz" -C "C:\tmp"
    }
}

function Setup-Kafka {
    param(
        [string]$Version = "kafka_2.13-3.5.0",
        [string]$Bucket
    )
    
    $isWin2016 = (Get-WmiObject -Class Win32_OperatingSystem).Caption -match "2016"
    
    Copy-S3Object -BucketName $Bucket -Key "kafka/$Version.tgz" -LocalFile "C:\tmp\$Version.tgz"
    
    if ($isWin2016) {
        $originalLocation = Get-Location
        try {
            Set-Location "C:\tmp"
            & "C:\Program Files\Git\usr\bin\gzip.exe" -d "$Version.tgz"
            & "C:\Program Files\Git\usr\bin\tar.exe" -xf "$Version.tar"
        } finally {
            Set-Location $originalLocation
        }
    } else {
        tar -xzf "C:\tmp\$Version.tgz" -C "C:\tmp"
    }
    
    $kafkaDir = "C:\tmp\$Version"
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
    
    & "$TomcatDir\bin\startup.bat"
    Start-Sleep -Seconds 10
}

function Stop-Tomcat {
    param([string]$TomcatDir)
    
    $jdkPath = Get-ChildItem -Path "C:\tmp\jdk17" -Directory | Select-Object -First 1 -ExpandProperty FullName
    $TomcatDir = $TomcatDir.TrimEnd('\')
    $env:JAVA_HOME = $jdkPath
    $env:CATALINA_HOME = $TomcatDir
    $env:CATALINA_BASE = $TomcatDir

    & "$TomcatDir\bin\shutdown.bat"
    Remove-Item -Path "env:CATALINA_OPTS", "env:CATALINA_BASE", "env:CATALINA_HOME", "env:JAVA_HOME" -ErrorAction SilentlyContinue
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
    
    $majorVersion = ($Version -split '-')[1] -split '\.' | Select-Object -First 1
    
    if ([int]$majorVersion -lt 4) {
        Start-Process -FilePath "$KafkaDir\bin\windows\zookeeper-server-start.bat" -ArgumentList "$KafkaDir\config\zookeeper.properties"
        Start-Sleep -Seconds 10
        $configFile = "$KafkaDir\config\server.properties"
    } else {
        $configFile = "$KafkaDir\config\controller.properties"
        $uuid = (& "$KafkaDir\bin\windows\kafka-storage.bat" random-uuid) | Select-Object -Last 1
        $uuid = $uuid.Trim()
        & "$KafkaDir\bin\windows\kafka-storage.bat" format --cluster-id $uuid --config $configFile --standalone
    }
    
    if ($Port) {
        $env:KAFKA_JMX_OPTS = "-Dcom.sun.management.jmxremote.port=$Port -Dcom.sun.management.jmxremote.authenticate=false -Dcom.sun.management.jmxremote.ssl=false"
    }
    
    Start-Process -FilePath "$KafkaDir\bin\windows\kafka-server-start.bat" -ArgumentList $configFile
    Start-Sleep -Seconds 10
}

function Stop-Kafka {
    param(
        [string]$KafkaDir,
        [string]$Version
    )

    $jdkPath = Get-ChildItem -Path "C:\tmp\jdk17" -Directory | Select-Object -First 1 -ExpandProperty FullName
    $env:JAVA_HOME = $jdkPath
    
    & "$KafkaDir\bin\windows\kafka-server-stop.bat"
    
    $majorVersion = ($Version -split '-')[1] -split '\.' | Select-Object -First 1
    if ([int]$majorVersion -lt 4) {
        & "$KafkaDir\bin\windows\zookeeper-server-stop.bat"
    }
    
    Remove-Item -Path "C:\tmp\zookeeper", "C:\tmp\kafka-logs", "C:\tmp\kraft-controller-logs" -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item -Path "env:KAFKA_JMX_OPTS", "env:JAVA_HOME", "env:KAFKA_HEAP_OPTS" -ErrorAction SilentlyContinue
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