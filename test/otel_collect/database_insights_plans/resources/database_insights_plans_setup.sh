#!/bin/bash
set -euo pipefail

# Adds the pg_store_plans library to the PostgreSQL instance that
# ../../database_insights/resources/database_insights_setup.sh prepared.
#
# The library is built and installed, but it is NOT preloaded and CREATE EXTENSION is
# NOT run. The test preloads it and restarts PostgreSQL while the agent is running
# (database_insights_plans_preload.sh), then runs CREATE EXTENSION, to check that plan
# collection resumes without an agent restart. The script also creates the
# non-superuser role that runs the workload.

echo "=== [1/4] Installing build dependencies ==="
if type -P yum >/dev/null 2>&1; then
    # Name of the installed server package: postgresqlNN-server on Amazon Linux 2023, postgresql-server on
    # RHEL-family distributions. Its -devel package has the server headers and the PGXS makefiles.
    # On Amazon Linux 2023 it also pulls in pg_config (postgresqlNN-private-devel), clang and llvm.
    SERVER_PKG=$(rpm -qa --qf '%{NAME}\n' 'postgresql*-server' | sort -V | tail -1)
    if [[ -z "$SERVER_PKG" ]]; then
        echo "ERROR: no postgresql*-server package is installed"
        exit 1
    fi
    # gcc depends on amazon-rpm-config on Amazon Linux 2023, which ships the -specs files that the
    # PGXS CFLAGS and LDFLAGS refer to (/usr/lib/rpm/redhat/redhat-hardened-cc1 and others).
    sudo yum install -y gcc make git "${SERVER_PKG}-devel"
elif type -P apt-get >/dev/null 2>&1; then
    # postgresql-server-dev-NN is in the universe component of Ubuntu. Enable it if the image does not.
    PG_MAJOR_DEB=$(ls /usr/lib/postgresql 2>/dev/null | sort -V | tail -1 || true)
    if [[ -z "$PG_MAJOR_DEB" ]]; then
        echo "ERROR: no PostgreSQL server is installed under /usr/lib/postgresql"
        exit 1
    fi
    APT_ENV=(env DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=a)
    if ! apt-cache policy "postgresql-server-dev-${PG_MAJOR_DEB}" | grep -q 'Candidate: [^(]'; then
        sudo "${APT_ENV[@]}" apt-get -o DPkg::Lock::Timeout=300 install -y software-properties-common
        sudo add-apt-repository -y universe
        sudo "${APT_ENV[@]}" apt-get -o DPkg::Lock::Timeout=300 update -y
    fi
    sudo "${APT_ENV[@]}" apt-get -o DPkg::Lock::Timeout=300 install -y \
        "postgresql-server-dev-${PG_MAJOR_DEB}" gcc make git
else
    # The generator only emits this test on al2023 (yum) and ubuntu-24 (apt-get).
    echo "ERROR: No supported package manager found (yum, apt-get)"
    exit 1
fi

PG_CONFIG=$(ls /usr/pgsql-*/bin/pg_config /usr/lib/postgresql/*/bin/pg_config 2>/dev/null | sort -V | tail -1 || true)
PG_CONFIG=${PG_CONFIG:-$(command -v pg_config)}
SERVER_MAJOR=$(sudo -u postgres psql -Atc "SHOW server_version" | cut -d. -f1)
BUILD_MAJOR=$("$PG_CONFIG" --version | sed -E 's/^PostgreSQL ([0-9]+).*/\1/')
if [[ "$SERVER_MAJOR" != "$BUILD_MAJOR" ]]; then
    echo "ERROR: pg_config ($PG_CONFIG) is for PostgreSQL $BUILD_MAJOR but the server is $SERVER_MAJOR"
    exit 1
fi

echo "=== [2/4] Building pg_store_plans for PostgreSQL $SERVER_MAJOR ==="
# Newest release first; the first release that compiles against this server wins.
# 1.10 has version guards for PostgreSQL 11 to 18 and 1.9 for 11 to 17. 1.8 stops at 16, so it is not tried.
BUILD_DIR=$(mktemp -d)
BUILT=""
for ver in 1.10 1.9; do
    rm -rf "$BUILD_DIR/src"
    if ! git clone --quiet --depth 1 --branch "$ver" https://github.com/ossc-db/pg_store_plans.git "$BUILD_DIR/src"; then
        continue
    fi
    # with_llvm=no skips bitcode generation, which needs clang on some distributions.
    if (cd "$BUILD_DIR/src" \
        && make USE_PGXS=1 PG_CONFIG="$PG_CONFIG" with_llvm=no \
        && sudo make USE_PGXS=1 PG_CONFIG="$PG_CONFIG" with_llvm=no install); then
        BUILT=$ver
        break
    fi
    echo "pg_store_plans $ver did not build against PostgreSQL $SERVER_MAJOR; trying the next release"
done
if [[ -z "$BUILT" ]]; then
    echo "ERROR: no pg_store_plans release built against PostgreSQL $SERVER_MAJOR"
    exit 1
fi
echo "Installed pg_store_plans $BUILT"

echo "=== [3/4] Creating the workload role ==="
# The receiver does not look up plans for statements owned by a superuser, so the
# workload must run as an ordinary role. It owns testdb so it can create the pgbench tables.
sudo -u postgres psql -v ON_ERROR_STOP=1 -c "CREATE ROLE dbi_app LOGIN PASSWORD 'dbi_app_password' NOSUPERUSER;"
sudo -u postgres psql -v ON_ERROR_STOP=1 -c "ALTER DATABASE testdb OWNER TO dbi_app;"
# testdb2 is the database whose extension the test creates BEFORE the library is preloaded.
sudo -u postgres psql -v ON_ERROR_STOP=1 -c "CREATE DATABASE testdb2 OWNER dbi_app;"
sudo -u postgres psql -v ON_ERROR_STOP=1 -c "GRANT CONNECT ON DATABASE testdb2 TO cw_monitor;"
HBA=$(sudo -u postgres psql -Atc "SHOW hba_file")
sudo sed -i '1i host all dbi_app 127.0.0.1/32 md5' "$HBA"
# pg_hba.conf needs a reload, not a restart. The restart is left to the test.
sudo -u postgres psql -Atc "SELECT pg_reload_conf()"

echo "=== [4/4] Verifying ==="
PRELOAD=$(sudo -u postgres psql -Atc "SHOW shared_preload_libraries")
if [[ "$PRELOAD" == *pg_store_plans* ]]; then
    echo "ERROR: pg_store_plans must not be preloaded yet (shared_preload_libraries = $PRELOAD)"
    exit 1
fi
AVAILABLE=$(sudo -u postgres psql -Atc "SELECT count(*) FROM pg_available_extensions WHERE name = 'pg_store_plans'")
if [[ "$AVAILABLE" != "1" ]]; then
    echo "ERROR: pg_store_plans is not available to CREATE EXTENSION"
    exit 1
fi
CREATED=$(sudo -u postgres psql -d testdb -Atc "SELECT count(*) FROM pg_extension WHERE extname = 'pg_store_plans'")
if [[ "$CREATED" != "0" ]]; then
    echo "ERROR: pg_store_plans must not be created in testdb before the agent starts"
    exit 1
fi
CREATED2=$(sudo -u postgres psql -d testdb2 -Atc "SELECT count(*) FROM pg_extension WHERE extname = 'pg_store_plans'")
if [[ "$CREATED2" != "0" ]]; then
    echo "ERROR: pg_store_plans must not be created in testdb2 before the agent starts"
    exit 1
fi
SUPER=$(sudo -u postgres psql -Atc "SELECT rolsuper FROM pg_roles WHERE rolname = 'dbi_app'")
if [[ "$SUPER" != "f" ]]; then
    echo "ERROR: dbi_app must not be a superuser"
    exit 1
fi
echo "=== pg_store_plans is installed, not preloaded and not created ==="
