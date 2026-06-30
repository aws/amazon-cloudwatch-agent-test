#!/bin/bash
set -euo pipefail

# Detect package manager and install MySQL with performance_schema support.
# Supports RPM-family (yum/dnf), Debian-family (apt-get), and SLES (zypper).
echo "=== [1/7] Installing MySQL ==="
if type -P yum >/dev/null 2>&1; then
    if type -P amazon-linux-extras >/dev/null 2>&1; then
        # AL2: install community MySQL from extras or default repo
        sudo amazon-linux-extras install -y mariadb10.5 2>/dev/null || true
        sudo yum install -y mysql-server mysql || {
            sudo yum install -y mariadb-server mariadb
        }
    else
        # Amazon Linux 2023, RHEL 8/9, Rocky Linux
        if sudo yum list available 'mysql-server' >/dev/null 2>&1; then
            sudo yum install -y mysql-server mysql
        elif sudo yum list available 'community-mysql-server' >/dev/null 2>&1; then
            sudo yum install -y community-mysql-server community-mysql
        else
            sudo yum install -y mariadb-server mariadb
        fi
    fi
elif type -P apt-get >/dev/null 2>&1; then
    # Ubuntu, Debian
    export DEBIAN_FRONTEND=noninteractive
    sudo apt-get update -y
    sudo apt-get install -y mysql-server mysql-client sysbench
elif type -P zypper >/dev/null 2>&1; then
    # SLES
    sudo zypper install -y mysql-server mysql-client
else
    echo "ERROR: No supported package manager found (yum, apt-get, zypper)"
    exit 1
fi

# Install sysbench for workload generation (if not already installed)
if ! type -P sysbench >/dev/null 2>&1; then
    if type -P yum >/dev/null 2>&1; then
        sudo yum install -y epel-release 2>/dev/null || true
        sudo yum install -y sysbench 2>/dev/null || \
            echo "WARNING: sysbench not available, will use mysqlslap instead"
    elif type -P zypper >/dev/null 2>&1; then
        sudo zypper install -y sysbench 2>/dev/null || true
    fi
fi

echo "=== [2/7] Starting MySQL service ==="
# Determine service name (mysqld on RPM-based, mysql on Debian, mariadb on SLES/AL2)
if systemctl list-unit-files | grep -q "^mysqld.service"; then
    MYSQL_SERVICE="mysqld"
elif systemctl list-unit-files | grep -q "^mysql.service"; then
    MYSQL_SERVICE="mysql"
elif systemctl list-unit-files | grep -q "^mariadb.service"; then
    MYSQL_SERVICE="mariadb"
else
    echo "ERROR: No MySQL/MariaDB service found"
    exit 1
fi
sudo systemctl enable "$MYSQL_SERVICE" && sudo systemctl start "$MYSQL_SERVICE"

echo "=== [3/7] Configuring MySQL for monitoring ==="
# Ensure the log directory exists and is writable by the mysql user.
sudo mkdir -p /var/log/mysql
sudo chown mysql:mysql /var/log/mysql 2>/dev/null || sudo chown mysql:adm /var/log/mysql

# Determine MySQL drop-in config directory.
if [[ -d /etc/mysql/conf.d ]]; then
    MYSQL_CONF="/etc/mysql/conf.d/monitoring.cnf"
elif [[ -d /etc/my.cnf.d ]]; then
    MYSQL_CONF="/etc/my.cnf.d/monitoring.cnf"
else
    MYSQL_CONF="/etc/my.cnf.d/monitoring.cnf"
    sudo mkdir -p /etc/my.cnf.d
fi

# performance_schema (and its statement/wait consumers) feeds DB Load and Top SQL.
# The error log feeds the DBI "server-logs" CloudWatch log group.
sudo tee "$MYSQL_CONF" << 'EOF'
[mysqld]
performance_schema = ON
performance_schema_instrument = 'statement/%=ON'
performance_schema_instrument = 'wait/%=ON'
performance_schema_consumer_events_statements_current = ON
performance_schema_consumer_events_waits_current = ON
performance_schema_consumer_events_statements_history = ON
log_error = /var/log/mysql/mysql-error.log
log_error_verbosity = 3
EOF

echo "=== [4/7] Restarting MySQL (config changes require restart) ==="
sudo systemctl restart "$MYSQL_SERVICE"

# Wait for MySQL to accept connections.
for i in $(seq 1 30); do
    if sudo mysqladmin ping --silent 2>/dev/null; then
        break
    fi
    sleep 1
done

echo "=== [5/7] Creating test database and monitoring user ==="
# Root auth differs across distros (socket auth vs empty password).
MYSQL_ROOT_CMD="sudo mysql"
if ! $MYSQL_ROOT_CMD -e "SELECT 1" >/dev/null 2>&1; then
    MYSQL_ROOT_CMD="mysql -u root"
fi

$MYSQL_ROOT_CMD << 'SQL'
CREATE DATABASE IF NOT EXISTS testdb;
CREATE USER IF NOT EXISTS 'cw_monitor'@'localhost' IDENTIFIED BY 'test_password';
GRANT SELECT ON performance_schema.* TO 'cw_monitor'@'localhost';
GRANT PROCESS ON *.* TO 'cw_monitor'@'localhost';
GRANT REPLICATION CLIENT ON *.* TO 'cw_monitor'@'localhost';
GRANT EXECUTE ON *.* TO 'cw_monitor'@'localhost';
GRANT SELECT ON testdb.* TO 'cw_monitor'@'localhost';
CREATE USER IF NOT EXISTS 'sysbench'@'localhost' IDENTIFIED BY 'sysbench';
GRANT ALL ON testdb.* TO 'sysbench'@'localhost';
FLUSH PRIVILEGES;
SQL

echo "=== [6/7] Verifying performance_schema is enabled ==="
$MYSQL_ROOT_CMD -e "SHOW VARIABLES LIKE 'performance_schema';"
$MYSQL_ROOT_CMD -e "SELECT COUNT(*) AS thread_count FROM performance_schema.threads WHERE processlist_command IS NOT NULL;"

echo "=== [7/7] Creating credentials file ==="
# pgpass-inspired passfile read by the MySQL receiver: host:port:database:user:password
sudo mkdir -p /opt/databaseinsights
echo 'localhost:3306:*:cw_monitor:test_password' | sudo tee /opt/databaseinsights/.mysql_credentials
sudo chmod 0600 /opt/databaseinsights/.mysql_credentials

echo "=== MySQL setup complete ==="
