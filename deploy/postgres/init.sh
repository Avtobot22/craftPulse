#!/bin/sh
# Runs as a PostgreSQL superuser: from /docker-entrypoint-initdb.d, in tests and on the stand.
set -eu

secret() {
    tr -d '\r\n' < "$1"
}

owner_password=$(secret "$CRAFTPULSE_OWNER_PASSWORD_FILE")
app_password=$(secret "$CRAFTPULSE_APP_PASSWORD_FILE")
backup_password=$(secret "$CRAFTPULSE_BACKUP_PASSWORD_FILE")

psql -v ON_ERROR_STOP=1 --username "${POSTGRES_USER:-postgres}" --dbname postgres \
    -v owner_password="$owner_password" \
    -v app_password="$app_password" \
    -v backup_password="$backup_password" <<'SQL'
CREATE ROLE craftpulse_owner LOGIN PASSWORD :'owner_password';
CREATE ROLE craftpulse_app LOGIN NOBYPASSRLS PASSWORD :'app_password';
CREATE ROLE craftpulse_backup LOGIN NOBYPASSRLS PASSWORD :'backup_password';

CREATE DATABASE craftpulse
    OWNER craftpulse_owner
    TEMPLATE template0
    ENCODING 'UTF8'
    LOCALE_PROVIDER builtin
    BUILTIN_LOCALE 'C.UTF-8';
ALTER DATABASE craftpulse SET timezone TO 'UTC';
REVOKE ALL ON DATABASE craftpulse FROM PUBLIC;
GRANT CONNECT ON DATABASE craftpulse TO craftpulse_app, craftpulse_backup;

\connect craftpulse
CREATE EXTENSION btree_gist;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
GRANT USAGE ON SCHEMA public TO craftpulse_app, craftpulse_backup;
ALTER DEFAULT PRIVILEGES FOR ROLE craftpulse_owner IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO craftpulse_app;
ALTER DEFAULT PRIVILEGES FOR ROLE craftpulse_owner IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO craftpulse_app;
ALTER DEFAULT PRIVILEGES FOR ROLE craftpulse_owner IN SCHEMA public
    GRANT SELECT ON TABLES TO craftpulse_backup;
SQL
