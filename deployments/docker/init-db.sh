#!/bin/sh
set -e
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-'EOSQL'
SELECT 'CREATE DATABASE heimdall_datasets' WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'heimdall_datasets')\gexec
SELECT 'CREATE DATABASE heimdall_scope_service' WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'heimdall_scope_service')\gexec
EOSQL
