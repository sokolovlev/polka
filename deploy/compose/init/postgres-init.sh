#!/bin/sh
# Creates one database and one owner role per service (spec 7: database per
# service, each user without rights on other databases). Runs once, on an
# empty data directory, via /docker-entrypoint-initdb.d.
set -eu

create() { # <database> <role> <password>
  psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres \
    -v db="$1" -v role="$2" -v pass="$3" <<'SQL'
CREATE ROLE :"role" LOGIN PASSWORD :'pass';
CREATE DATABASE :"db" OWNER :"role";
REVOKE ALL ON DATABASE :"db" FROM PUBLIC;
SQL
}

create auth          auth          "$POLKA_AUTH_DB_PASSWORD"
create catalog       catalog       "$POLKA_CATALOG_DB_PASSWORD"
create orders        orders        "$POLKA_ORDERS_DB_PASSWORD"
create inventory     inventory     "$POLKA_INVENTORY_DB_PASSWORD"
create payments      payments      "$POLKA_PAYMENTS_DB_PASSWORD"
create notifications notifications "$POLKA_NOTIFICATIONS_DB_PASSWORD"

# Extensions that need superuser: citext (auth.users.email), ltree (catalog).
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname auth -c 'CREATE EXTENSION IF NOT EXISTS citext;'
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname catalog -c 'CREATE EXTENSION IF NOT EXISTS ltree;'
