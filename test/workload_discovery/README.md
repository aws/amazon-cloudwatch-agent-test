# Workload Discovery Tests

## Local Debugging with Scripts

The `scripts_unix.sh` and `scripts_windows.ps1` files can be used directly for local debugging and testing.

### Unix/Linux (scripts_unix.sh)

```bash
# Install Java 17
./scripts_unix.sh install_java17 <bucket-name>

# Create test JAR
./scripts_unix.sh setup_jar test.jar "Main-Class=Main"

# Start JVM without JMX
jvm_pid=$(./scripts_unix.sh spin_up_jvm /tmp/test.jar)

# Start JVM with JMX port
jvm_pid=$(./scripts_unix.sh spin_up_jvm /tmp/test.jar 2030)

# Stop JVM
./scripts_unix.sh tear_down_jvm $jvm_pid

# Setup and run Kafka
./scripts_unix.sh setup_kafka kafka_2.13-3.5.0 <bucket-name>
kafka_pid=$(./scripts_unix.sh spin_up_kafka /tmp/kafka/kafka_2.13-3.5.0 kafka_2.13-3.5.0)
kafka_pid=$(./scripts_unix.sh spin_up_kafka /tmp/kafka/kafka_2.13-3.5.0 kafka_2.13-3.5.0 9999)
./scripts_unix.sh tear_down_kafka $kafka_pid /tmp/kafka/kafka_2.13-3.5.0 kafka_2.13-3.5.0

# Setup and run Tomcat
./scripts_unix.sh setup_tomcat apache-tomcat-9.0.110 <bucket-name>
tomcat_pid=$(./scripts_unix.sh spin_up_tomcat /tmp/tomcat/apache-tomcat-9.0.110)
tomcat_pid=$(./scripts_unix.sh spin_up_tomcat /tmp/tomcat/apache-tomcat-9.0.110 1080)
./scripts_unix.sh tear_down_tomcat $tomcat_pid /tmp/tomcat/apache-tomcat-9.0.110

# NVIDIA setup
./scripts_unix.sh setup_nvidia_device
./scripts_unix.sh install_nvidia_driver <bucket-name>
./scripts_unix.sh uninstall_nvidia

# Check CloudWatch Agent workload status
sudo /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl -a status-with-workloads
```

### Windows (scripts_windows.ps1)

```powershell
# Install Java 17
powershell -File C:\scripts.ps1 Install-Java17 -Bucket <bucket-name>

# Create test JAR
powershell -File C:\scripts.ps1 Setup-Jar test.jar "Main-Class=Main"

# Start JVM without JMX
$jvmPid = powershell -File C:\scripts.ps1 Start-JVM -JarPath C:\tmp\test.jar

# Start JVM with JMX port
$jvmPid = powershell -File C:\scripts.ps1 Start-JVM -JarPath C:\tmp\test.jar -Port 2030

# Stop JVM
powershell -File C:\scripts.ps1 Stop-JVM -ProcessId $jvmPid

# Setup and run Kafka
powershell -File C:\scripts.ps1 Setup-Kafka -Version kafka_2.13-3.5.0 -Bucket <bucket-name>
$kafkaPid = powershell -File C:\scripts.ps1 Start-Kafka -KafkaDir C:\tmp\kafka\kafka_2.13-3.5.0 -Version kafka_2.13-3.5.0
$kafkaPid = powershell -File C:\scripts.ps1 Start-Kafka -KafkaDir C:\tmp\kafka\kafka_2.13-3.5.0 -Version kafka_2.13-3.5.0 -Port 9999
powershell -File C:\scripts.ps1 Stop-Kafka -ProcessId $kafkaPid -KafkaDir C:\tmp\kafka\kafka_2.13-3.5.0 -Version kafka_2.13-3.5.0

# Setup and run Tomcat
powershell -File C:\scripts.ps1 Setup-Tomcat -Version apache-tomcat-9.0.110 -Bucket <bucket-name>
$tomcatPid = powershell -File C:\scripts.ps1 Start-Tomcat -TomcatDir C:\tmp\tomcat\apache-tomcat-9.0.110
$tomcatPid = powershell -File C:\scripts.ps1 Start-Tomcat -TomcatDir C:\tmp\tomcat\apache-tomcat-9.0.110 -Port 1080
powershell -File C:\scripts.ps1 Stop-Tomcat -ProcessId $tomcatPid -TomcatDir C:\tmp\tomcat\apache-tomcat-9.0.110

# NVIDIA setup
powershell -File C:\scripts.ps1 Install-NvidiaDriver -Bucket <bucket-name>
powershell -File C:\scripts.ps1 Uninstall-NvidiaDriver

# Check CloudWatch Agent workload status
powershell -File "C:\Program Files\Amazon\AmazonCloudWatchAgent\amazon-cloudwatch-agent-ctl.ps1" -a status-with-workloads
```
