#!/usr/bin/env bash
/usr/bin/mariadb --user=root --password="$MARIADB_ROOT_PASSWORD" <<-EOSQL
    CREATE DATABASE IF NOT EXISTS testing;
EOSQL

if [ -n "$MARIADB_USER" ]; then
/usr/bin/mariadb --user=root --password="$MARIADB_ROOT_PASSWORD" <<-EOSQL
    GRANT ALL PRIVILEGES ON \`testing%\`.* TO '$MARIADB_USER'@'%';
EOSQL
fi
