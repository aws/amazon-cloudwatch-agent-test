#!/bin/sh

# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: MIT

install_java17() {
    BUCKET="$1"

    # Override existing JDK11 installation from AMI (TODO: Remove when AMI uses JDK17 by default)
    if type -P dnf >/dev/null 2>&1; then
        sudo dnf install -y java-17-amazon-corretto-devel 2>/dev/null
    fi
    
    if type -P yum >/dev/null 2>&1; then
        sudo yum install -y java-17-amazon-corretto-devel 2>/dev/null
    fi

    if type -P apt-get >/dev/null 2>&1; then
        sudo apt-get install -y java-17-amazon-corretto-jdk 2>/dev/null
    fi
    
    if type -P zypper >/dev/null 2>&1; then
        sudo zypper install -y java-17-amazon-corretto-devel 2>/dev/null
    fi

    # Main JDK17 installation
    ARCH=$(uname -m)
    if [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ]; then
        S3_KEY="jdk17/linux/amazon-corretto-17-aarch64-linux-jdk.tar.gz"
        JAVA_DIR="/tmp/amazon-corretto-17.0.16.8.1-linux-aarch64"
    else
        S3_KEY="jdk17/linux/amazon-corretto-17-x64-linux-jdk.tar.gz"
        JAVA_DIR="/tmp/amazon-corretto-17.0.16.8.1-linux-x64"
    fi
    
    aws s3 cp "s3://$BUCKET/$S3_KEY" /tmp/jdk17.tar.gz
    sudo tar -xzf /tmp/jdk17.tar.gz -C /tmp
    sudo rm -f /usr/local/bin/java /usr/local/bin/javac /usr/local/bin/jar
    sudo ln -sf "$JAVA_DIR/bin/java" /usr/local/bin/java
    sudo ln -sf "$JAVA_DIR/bin/javac" /usr/local/bin/javac
    sudo ln -sf "$JAVA_DIR/bin/jar" /usr/local/bin/jar
    export JAVA_HOME="$JAVA_DIR"
    export PATH="$JAVA_DIR/bin:$PATH"
}

setup_jar() {
    JAR_NAME="${1:-test.jar}"
    shift 
    
    sudo mkdir -p /tmp/jvm-compile
    sudo chmod 777 /tmp/jvm-compile
    sudo tee /tmp/jvm-compile/Main.java > /dev/null << 'EOF'
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
EOF

    sudo chmod 666 /tmp/jvm-compile/Main.java
    javac /tmp/jvm-compile/Main.java
    sudo mkdir -p /tmp/jvm-compile/META-INF
    echo "Manifest-Version: 1.0" | sudo tee /tmp/jvm-compile/META-INF/MANIFEST.MF > /dev/null

    for arg in "$@"; do
        if echo "$arg" | grep -q "="; then
            key=$(echo "$arg" | cut -d'=' -f1)
            value=$(echo "$arg" | cut -d'=' -f2-)
            echo "$key: $value" | sudo tee -a /tmp/jvm-compile/META-INF/MANIFEST.MF > /dev/null
        fi
    done
    
    cd /tmp/jvm-compile
    jar cfm "/tmp/$JAR_NAME" META-INF/MANIFEST.MF *.class
    sudo rm -rf /tmp/jvm-compile
}

setup_tomcat() {
    VERSION="${1:-apache-tomcat-9.0.110}"
    BUCKET="$2"
    aws s3 cp "s3://$BUCKET/tomcat/$VERSION.tar.gz" "/tmp/tomcat/$VERSION.tar.gz"
    sudo tar -xzf "/tmp/tomcat/$VERSION.tar.gz" -C /tmp/tomcat
    TOMCAT_DIR="/tmp/tomcat/$VERSION"
    sudo chmod -R 755 "$TOMCAT_DIR"
}

setup_kafka() {
    VERSION="${1:-kafka_2.13-3.5.0}"
    BUCKET="$2"
    aws s3 cp "s3://$BUCKET/kafka/$VERSION.tgz" "/tmp/kafka/$VERSION.tgz"
    sudo tar -xzf "/tmp/kafka/$VERSION.tgz" -C /tmp/kafka
    KAFKA_DIR="/tmp/kafka/$VERSION"
    sudo chmod -R 755 "$KAFKA_DIR"
}

spin_up_jvm() {
    JAR_PATH="${1:-/tmp/test.jar}"
    PORT="$2"
    
    if [ -n "$PORT" ]; then
        java -Dcom.sun.management.jmxremote \
             -Dcom.sun.management.jmxremote.port="$PORT" \
             -Dcom.sun.management.jmxremote.local.only=false \
             -Dcom.sun.management.jmxremote.authenticate=false \
             -Dcom.sun.management.jmxremote.ssl=false \
             -Dcom.sun.management.jmxremote.rmi.port="$PORT" \
             -Dcom.sun.management.jmxremote.host=localhost \
             -Djava.rmi.server.hostname=localhost \
             -jar "$JAR_PATH" >/dev/null 2>&1 &
        PID=$!
        echo $PID
    else
        java -jar "$JAR_PATH" >/dev/null 2>&1 &
        echo $!
    fi
}

tear_down_jvm() {
    PID="$1"
    kill "$PID" 2>/dev/null
    while kill -0 "$PID" 2>/dev/null; do
        sleep 0.1
    done
}

spin_up_tomcat() {
    TOMCAT_DIR="$1"
    PORT="$2"
    
    export CATALINA_BASE="$TOMCAT_DIR"
    if [ -n "$PORT" ]; then
        export CATALINA_OPTS="-Dcom.sun.management.jmxremote.port=$PORT -Dcom.sun.management.jmxremote.authenticate=false -Dcom.sun.management.jmxremote.ssl=false"
    fi
    
    sudo -E "$TOMCAT_DIR/bin/startup.sh" >/dev/null 2>&1
    
    sleep 5
    TOMCAT_PID=$(pgrep -f "$TOMCAT_DIR" | head -1)
    echo "$TOMCAT_PID"
}

tear_down_tomcat() {
    PID="$1"
    TOMCAT_DIR="$2"
    
    sudo "$TOMCAT_DIR/bin/shutdown.sh" >/dev/null 2>&1
    sudo pkill -f tomcat 2>/dev/null
    
    if [ -n "$PID" ]; then
        kill "$PID" 2>/dev/null
        while kill -0 "$PID" 2>/dev/null; do
            sleep 0.1
        done
    fi
    
    unset CATALINA_OPTS CATALINA_BASE
}

spin_up_kafka() {
    KAFKA_DIR="$1"
    VERSION="$2"
    PORT="$3"

    sudo rm -rf /tmp/zookeeper /tmp/kafka-logs /tmp/kraft-controller-logs 2>/dev/null
    
    MAJOR_VERSION=$(echo "$VERSION" | cut -d'-' -f2 | cut -d'.' -f1)
    if [ "$MAJOR_VERSION" -lt 4 ]; then
        sudo -E "$KAFKA_DIR/bin/zookeeper-server-start.sh" "$KAFKA_DIR/config/zookeeper.properties" >/dev/null 2>&1 &
        CONFIG_FILE="$KAFKA_DIR/config/server.properties"
    else
        CONFIG_FILE="$KAFKA_DIR/config/controller.properties"
        UUID=$("$KAFKA_DIR/bin/kafka-storage.sh" random-uuid)
        "$KAFKA_DIR/bin/kafka-storage.sh" format -t "$UUID" -c "$CONFIG_FILE" --standalone >/dev/null 2>&1
    fi
    
    if [ -n "$PORT" ]; then
        export KAFKA_JMX_OPTS="-Dcom.sun.management.jmxremote.port=$PORT -Dcom.sun.management.jmxremote.authenticate=false -Dcom.sun.management.jmxremote.ssl=false"
    fi
    
    sudo -E "$KAFKA_DIR/bin/kafka-server-start.sh" "$CONFIG_FILE" >/dev/null 2>&1 &
    KAFKA_PID=$!
    
    sleep 5
    echo "$KAFKA_PID"
}

tear_down_kafka() {
    PID="$1"
    KAFKA_DIR="$2"
    VERSION="$3"
    
    sudo "$KAFKA_DIR/bin/kafka-server-stop.sh" >/dev/null 2>&1
    
    MAJOR_VERSION=$(echo "$VERSION" | cut -d'-' -f2 | cut -d'.' -f1)
    if [ "$MAJOR_VERSION" -lt 4 ]; then
        sudo "$KAFKA_DIR/bin/zookeeper-server-stop.sh" >/dev/null 2>&1
    fi
    
    sudo pkill -f kafka 2>/dev/null
    
    if [ -n "$PID" ]; then
        kill "$PID" 2>/dev/null
        while kill -0 "$PID" 2>/dev/null; do
            sleep 0.1
        done
    fi
    
    unset KAFKA_JMX_OPTS
}

# Mimics the registration of an NVIDIA device since actual registration requires reboot
setup_nvidia_device() {
    sudo mknod /dev/nvidia0 c 195 0
    sudo chmod 666 /dev/nvidia0
}

install_nvidia_driver() {
    BUCKET="$1"    
    aws s3 cp "s3://$BUCKET/nvidia/linux/latest/NVIDIA-Linux-x86_64-580.95.05-grid-aws.run" /tmp/nvidia-driver.run
    sudo chmod +x /tmp/nvidia-driver.run
    sudo /tmp/nvidia-driver.run --silent --no-questions --ui=none --no-kernel-module
}

uninstall_nvidia() {
    sudo /tmp/nvidia-driver.run --uninstall --silent --no-questions --ui=none
    sudo rm -rf /dev/nvidia0
    sudo rm -rf /tmp/nvidia-driver.run
}

if [ $# -gt 0 ]; then
    FUNCTION_NAME="$1"
    shift
    "$FUNCTION_NAME" "$@"
fi
