#!/bin/bash
set -euo pipefail

# Install PostgreSQL (14+) with pg_stat_statements support.
# Supports RPM-family (yum/dnf), Debian-family (apt-get), and SLES (zypper).
echo "=== [1/7] Installing PostgreSQL ==="
if type -P yum >/dev/null 2>&1; then
    if type -P amazon-linux-extras >/dev/null 2>&1; then
        # Amazon Linux 2: default repo only has PG 9.2; use extras to get 14
        sudo amazon-linux-extras install postgresql14 -y
        sudo yum install -y postgresql-server postgresql-contrib
    else
        # Amazon Linux 2023, RHEL 8/9, Rocky Linux, Alma Linux
        # EL8/9 use module streams; enable the latest to ensure PG 14+.
        # No-op on AL2023 (no modules). || true prevents pipefail exit.
        PG_STREAM=$(sudo dnf module list postgresql 2>/dev/null | awk '/postgresql/ {print $2}' | sort -V | tail -1) || true
        if [[ -n "$PG_STREAM" ]]; then
            sudo dnf module reset postgresql -y 2>/dev/null || true
            sudo dnf module enable postgresql:"$PG_STREAM" -y 2>/dev/null || true
            sudo dnf clean metadata 2>/dev/null || true
        fi
        PG_SERVER_PKG=$(sudo yum list available 'postgresql*-server' 2>/dev/null | awk '/postgresql[0-9]+-server/ {print $1}' | sort -V | tail -1)
        if [[ -z "$PG_SERVER_PKG" ]]; then
            PG_SERVER_PKG=$(sudo yum list available 'postgresql-server' 2>/dev/null | awk '/postgresql-server/ {print $1}')
        fi
        if [[ -z "$PG_SERVER_PKG" ]]; then
            echo "ERROR: No postgresql*-server package found in available repositories"
            exit 1
        fi
        PG_CONTRIB_PKG=$(echo "$PG_SERVER_PKG" | sed 's/-server/-contrib/')
        echo "Installing packages: $PG_SERVER_PKG $PG_CONTRIB_PKG"
        sudo yum install -y "$PG_SERVER_PKG" "$PG_CONTRIB_PKG"
    fi
    sudo postgresql-setup --initdb 2>/dev/null || sudo postgresql-setup initdb
    PG_DATA="/var/lib/pgsql/data"
elif type -P apt-get >/dev/null 2>&1; then
    # Ubuntu, Debian
    sudo apt-get update -y
    sudo apt-get install -y postgresql postgresql-contrib
    PG_DATA=$(find /etc/postgresql -maxdepth 2 -name "main" -type d | sort -V | tail -1)
    if [[ -z "$PG_DATA" ]]; then
        echo "ERROR: Could not find PostgreSQL config directory"
        exit 1
    fi
elif type -P zypper >/dev/null 2>&1; then
    # SLES
    sudo zypper install -y postgresql-server postgresql-contrib
    if [[ ! -d "/var/lib/pgsql/data/base" ]]; then
        sudo su - postgres -c "initdb -D /var/lib/pgsql/data"
    fi
    PG_DATA="/var/lib/pgsql/data"
else
    echo "ERROR: No supported package manager found (yum, apt-get, zypper)"
    exit 1
fi

echo "=== [2/7] Starting PostgreSQL service ==="
sudo systemctl enable postgresql && sudo systemctl start postgresql

echo "=== [3/7] Configuring postgresql.conf for monitoring ==="
sudo mkdir -p /var/log/postgresql
sudo chown postgres:postgres /var/log/postgresql

sudo tee -a "$PG_DATA/postgresql.conf" << 'EOF'
shared_preload_libraries = 'pg_stat_statements'
track_activities = on
pg_stat_statements.max = 10000
pg_stat_statements.track = all
pg_stat_statements.track_utility = on
log_destination = 'stderr'
logging_collector = on
log_directory = '/var/log/postgresql'
log_filename = 'postgresql-%Y-%m-%d.log'
log_min_duration_statement = 50
EOF

# Insert rules at the top of pg_hba.conf (first-match-wins) for both IPv4 and IPv6
echo "=== [4/7] Configuring pg_hba.conf for monitoring user ==="
sudo sed -i '1i host all cw_monitor ::1/128 md5' "$PG_DATA/pg_hba.conf"
sudo sed -i '1i host all cw_monitor 127.0.0.1/32 md5' "$PG_DATA/pg_hba.conf"

# shared_preload_libraries requires a full restart (not just reload)
echo "=== [5/7] Restarting PostgreSQL ==="
sudo systemctl restart postgresql

echo "=== [6/7] Creating database, extension, and monitoring user ==="
sudo -u postgres createdb testdb
sudo -u postgres psql -d postgres -c "CREATE EXTENSION IF NOT EXISTS pg_stat_statements;"
sudo -u postgres psql -c "CREATE USER cw_monitor WITH PASSWORD 'test_password';"
sudo -u postgres psql -c "GRANT pg_monitor TO cw_monitor;"
sudo -u postgres psql -c "GRANT CONNECT ON DATABASE testdb TO cw_monitor;"

echo "=== [7/7] Creating pgpass file ==="
sudo mkdir -p /opt/databaseinsights
echo 'localhost:5432:*:cw_monitor:test_password' | sudo tee /opt/databaseinsights/.pgpass
sudo chmod 0600 /opt/databaseinsights/.pgpass

echo "=== PostgreSQL setup complete ==="
