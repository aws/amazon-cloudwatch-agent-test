#!/bin/bash
set -euo pipefail

# Preloads pg_store_plans and restarts PostgreSQL. The test runs this while the agent
# is already running, which is the order a customer follows when they add the
# extension to a server that Database Insights already monitors.
#
# PostgreSQL must be restarted for shared_preload_libraries to take effect. A reload is
# not enough. CREATE EXTENSION is NOT run here.

echo "=== [1/3] Preloading pg_store_plans ==="
# The last occurrence of a setting in postgresql.conf wins, so this overrides the
# shared_preload_libraries line that database_insights_setup.sh appended. In the
# configuration file the library list is one quoted, comma-separated string. Do not
# set it with ALTER SYSTEM SET shared_preload_libraries = 'a,b': that stores a single
# library named "a,b" and PostgreSQL then fails to start.
CONF=$(sudo -u postgres psql -Atc "SHOW config_file")
sudo tee -a "$CONF" << 'EOF2'
shared_preload_libraries = 'pg_stat_statements,pg_store_plans'
compute_query_id = on
pg_store_plans.track = all
EOF2

echo "=== [2/3] Restarting PostgreSQL ==="
sudo systemctl restart postgresql
for _ in $(seq 1 60); do
    if sudo -u postgres pg_isready -q; then
        break
    fi
    sleep 1
done
if ! sudo -u postgres pg_isready -q; then
    echo "ERROR: PostgreSQL did not accept connections within 60 seconds of the restart"
    exit 1
fi

echo "=== [3/3] Verifying ==="
PRELOAD=$(sudo -u postgres psql -Atc "SHOW shared_preload_libraries")
if [[ "$PRELOAD" != *pg_store_plans* ]]; then
    echo "ERROR: pg_store_plans is not preloaded (shared_preload_libraries = $PRELOAD)"
    exit 1
fi
CREATED=$(sudo -u postgres psql -d testdb -Atc "SELECT count(*) FROM pg_extension WHERE extname = 'pg_store_plans'")
if [[ "$CREATED" != "0" ]]; then
    echo "ERROR: pg_store_plans must not be created yet"
    exit 1
fi
echo "=== pg_store_plans is preloaded and PostgreSQL has restarted ==="
