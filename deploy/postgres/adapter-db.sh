#!/bin/sh
# Creates the database of one adapter (TZ 12.2, 12.5.9) as a PostgreSQL superuser, after init.sh:
#   adapter-db.sh <name>   with ADAPTER_OWNER_PASSWORD_FILE and ADAPTER_PASSWORD_FILE set.
# Owner adapter_<name>_owner runs `migrate`; adapter_<name> runs the process with DML under RLS.
set -eu

name=${1:?usage: adapter-db.sh <name>}
case $name in
    *[!a-z0-9]* | '') echo "adapter name must be lowercase letters and digits" >&2; exit 2 ;;
esac

secret() {
    tr -d '\r\n' < "$1"
}

owner_password=$(secret "$ADAPTER_OWNER_PASSWORD_FILE")
app_password=$(secret "$ADAPTER_PASSWORD_FILE")
owner="adapter_${name}_owner"
app="adapter_${name}"

psql -v ON_ERROR_STOP=1 --username "${POSTGRES_USER:-postgres}" --dbname postgres \
    -v owner="$owner" -v app="$app" \
    -v owner_password="$owner_password" -v app_password="$app_password" <<'SQL'
CREATE ROLE :"owner" LOGIN PASSWORD :'owner_password';
CREATE ROLE :"app" LOGIN NOBYPASSRLS PASSWORD :'app_password';

CREATE DATABASE :"app"
    OWNER :"owner"
    TEMPLATE template0
    ENCODING 'UTF8'
    LOCALE_PROVIDER builtin
    BUILTIN_LOCALE 'C.UTF-8';
ALTER DATABASE :"app" SET timezone TO 'UTC';
REVOKE ALL ON DATABASE :"app" FROM PUBLIC;
GRANT CONNECT ON DATABASE :"app" TO :"app", craftpulse_backup;

\connect :"app"
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
CREATE SCHEMA adapter AUTHORIZATION :"owner";
GRANT USAGE ON SCHEMA adapter TO :"app", craftpulse_backup;
ALTER DEFAULT PRIVILEGES FOR ROLE :"owner" IN SCHEMA adapter
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO :"app";
ALTER DEFAULT PRIVILEGES FOR ROLE :"owner" IN SCHEMA adapter
    GRANT SELECT ON TABLES TO craftpulse_backup;
-- The process reads the schema version for /readyz and changes nothing else outside schema adapter.
ALTER DEFAULT PRIVILEGES FOR ROLE :"owner" IN SCHEMA public
    GRANT SELECT ON TABLES TO :"app", craftpulse_backup;
SQL
