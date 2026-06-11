#!/bin/bash
set -euo pipefail

echo "=== [1/8] Installing PostgreSQL ==="
# AL2023 uses versioned package names (e.g. postgresql16-server).
# Find the latest available postgresql*-server package.
PG_SERVER_PKG=$(dnf list available 'postgresql*-server' 2>/dev/null | awk '/postgresql[0-9]+-server/ {print $1}' | sort -V | tail -1)
PG_CONTRIB_PKG=$(echo "$PG_SERVER_PKG" | sed 's/-server/-contrib/')
echo "Installing packages: $PG_SERVER_PKG $PG_CONTRIB_PKG"
sudo dnf install -y "$PG_SERVER_PKG" "$PG_CONTRIB_PKG"

echo "=== [2/8] Initializing database cluster ==="
sudo postgresql-setup --initdb

echo "=== [3/8] Starting PostgreSQL service ==="
sudo systemctl enable postgresql && sudo systemctl start postgresql

echo "=== [4/8] Configuring postgresql.conf for monitoring ==="
# Use a consistent log directory so the agent config works across distros
sudo mkdir -p /var/log/postgresql
sudo chown postgres:postgres /var/log/postgresql

sudo tee -a /var/lib/pgsql/data/postgresql.conf << 'EOF'
shared_preload_libraries = 'pg_stat_statements'
track_activities = on
compute_query_id = on
pg_stat_statements.max = 10000
pg_stat_statements.track = all
pg_stat_statements.track_utility = on
pg_stat_statements.track_planning = on
log_destination = 'stderr'
logging_collector = on
log_directory = '/var/log/postgresql'
log_filename = 'postgresql-%Y-%m-%d.log'
log_min_duration_statement = 0
EOF

echo "=== [5/8] Configuring pg_hba.conf for monitoring user ==="
# Insert our rule at the top of pg_hba.conf (before default ident rules).
# PostgreSQL uses first-match-wins, so our line must come first.
sudo sed -i '1i host all cw_monitor 127.0.0.1/32 scram-sha-256' /var/lib/pgsql/data/pg_hba.conf

echo "=== [6/8] Restarting PostgreSQL (shared_preload_libraries requires restart) ==="
sudo systemctl restart postgresql

echo "=== [7/8] Creating database, extension, and monitoring user ==="
sudo -u postgres createdb testdb
sudo -u postgres psql -d postgres -c "CREATE EXTENSION IF NOT EXISTS pg_stat_statements;"
sudo -u postgres psql -c "CREATE USER cw_monitor WITH PASSWORD 'test_password';"
sudo -u postgres psql -c "GRANT pg_monitor TO cw_monitor;"
sudo -u postgres psql -c "GRANT CONNECT ON DATABASE testdb TO cw_monitor;"

echo "=== [8/8] Creating pgpass file ==="
sudo mkdir -p /opt/databaseinsights
echo 'localhost:5432:*:cw_monitor:test_password' | sudo tee /opt/databaseinsights/.pgpass
sudo chmod 0600 /opt/databaseinsights/.pgpass

echo "=== PostgreSQL setup complete ==="
