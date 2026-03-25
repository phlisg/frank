#!/usr/bin/env bash
# frank/scripts/init.sh — interactive wizard to create frank.yaml
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/config.sh"

# Parse arguments
FROM_SAIL=""
SAIL_FILE=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --from-sail) FROM_SAIL=1; shift ;;
        -f) SAIL_FILE="$2"; shift 2 ;;
        *)  echo "Unknown argument: $1" >&2; exit 1 ;;
    esac
done

# Handle Sail import
if [ -n "$FROM_SAIL" ]; then
    SAIL_FILE="${SAIL_FILE:-./docker-compose.yml}"
    if [ ! -f "$SAIL_FILE" ]; then
        echo "Error: Sail compose file not found: $SAIL_FILE" >&2
        exit 1
    fi
    exec "$SCRIPT_DIR/sail-import.sh" -f "$SAIL_FILE"
fi

# Detect existing Frank project (migration)
if [ -f "docker-compose.yml" ] && [ -d "frank" ]; then
    echo "Detected existing Frank project."
    read -rp "Import settings from current setup? (Y/n): " import_existing
    if [ "${import_existing:-y}" != "n" ]; then
        existing_php=$(grep -oP 'php\K[0-9]+\.[0-9]+' Dockerfile 2>/dev/null | head -1 || echo "")
        echo "  Found PHP version: ${existing_php:-unknown}"
    fi
fi

echo ""
echo "🏕️  Frank — Laravel Development Environment"
echo "============================================"
echo ""

# PHP version
echo "PHP version:"
echo "  1) 8.5 (default)"
echo "  2) 8.4"
echo "  3) 8.3"
echo "  4) 8.2"
read -rp "Choose [1]: " php_choice
case "${php_choice:-1}" in
    1) PHP_VERSION="8.5" ;;
    2) PHP_VERSION="8.4" ;;
    3) PHP_VERSION="8.3" ;;
    4) PHP_VERSION="8.2" ;;
    *) PHP_VERSION="8.5" ;;
esac

echo ""

# Runtime
echo "PHP runtime:"
echo "  1) FrankenPHP (default — single container, built-in web server)"
echo "  2) PHP-FPM + Nginx (traditional, matches shared hosting)"
read -rp "Choose [1]: " runtime_choice
case "${runtime_choice:-1}" in
    1) RUNTIME="frankenphp" ;;
    2) RUNTIME="fpm" ;;
    *) RUNTIME="frankenphp" ;;
esac

echo ""

# Database
echo "Database:"
echo "  1) PostgreSQL (default)"
echo "  2) MySQL"
echo "  3) MariaDB"
echo "  4) SQLite"
echo "  5) None"
read -rp "Choose [1]: " db_choice
case "${db_choice:-1}" in
    1) DB_SERVICE="pgsql" ;;
    2) DB_SERVICE="mysql" ;;
    3) DB_SERVICE="mariadb" ;;
    4) DB_SERVICE="sqlite" ;;
    5) DB_SERVICE="" ;;
    *) DB_SERVICE="pgsql" ;;
esac

echo ""

# Additional services
echo "Additional services (comma-separated, or Enter for defaults):"
echo "  Available: redis, meilisearch, memcached, mailpit"
echo "  Default:   mailpit"
read -rp "Services [mailpit]: " extra_services
extra_services="${extra_services:-mailpit}"

# Build services list
SERVICES=()
[ -n "$DB_SERVICE" ] && SERVICES+=("$DB_SERVICE")
IFS=',' read -ra EXTRA <<< "$extra_services"
for svc in "${EXTRA[@]}"; do
    svc=$(echo "$svc" | xargs)  # trim whitespace
    [ -n "$svc" ] && SERVICES+=("$svc")
done

echo ""

# Laravel version
echo "Laravel version:"
echo "  1) Latest (default)"
echo "  2) LTS"
echo "  3) Specific version"
read -rp "Choose [1]: " laravel_choice
case "${laravel_choice:-1}" in
    1) LARAVEL_VERSION="latest" ;;
    2) LARAVEL_VERSION="lts" ;;
    3) read -rp "Version (e.g., 11.*): " LARAVEL_VERSION ;;
    *) LARAVEL_VERSION="latest" ;;
esac

echo ""
echo "Configuration:"
echo "  PHP:      $PHP_VERSION ($RUNTIME)"
echo "  Laravel:  $LARAVEL_VERSION"
echo "  Services: ${SERVICES[*]}"
echo ""
read -rp "Write frank.yaml? (Y/n): " confirm
if [ "${confirm:-y}" = "n" ]; then
    echo "Aborted."
    exit 0
fi

# Write frank.yaml
SERVICES_YAML=""
for svc in "${SERVICES[@]}"; do
    SERVICES_YAML="${SERVICES_YAML}  - ${svc}\n"
done

cat > "$PROJECT_DIR/frank.yaml" << EOF
version: 1

php:
  version: "$PHP_VERSION"
  runtime: "$RUNTIME"

laravel:
  version: "$LARAVEL_VERSION"

services:
$(echo -e "$SERVICES_YAML")
EOF

echo "✅ frank.yaml written."

# Run generate
echo ""
"$SCRIPT_DIR/generate.sh"
