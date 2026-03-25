#!/usr/bin/env bash
# frank/scripts/sail-import.sh — parse Sail docker-compose.yml into frank.yaml
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/config.sh"

SAIL_FILE=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        -f) SAIL_FILE="$2"; shift 2 ;;
        *)  SAIL_FILE="$1"; shift ;;
    esac
done

SAIL_FILE="${SAIL_FILE:-./docker-compose.yml}"

if [ ! -f "$SAIL_FILE" ]; then
    echo "Error: Sail compose file not found: $SAIL_FILE" >&2
    exit 1
fi

echo "🔍 Parsing Sail compose file: $SAIL_FILE"

# Extract PHP version from Sail image or build context
PHP_VERSION=$(yq eval '.services.*.build.context // ""' "$SAIL_FILE" | grep -oP 'runtimes/\K[0-9.]+' | head -1 || echo "")
if [ -z "$PHP_VERSION" ]; then
    PHP_VERSION=$(yq eval '.services.*.image // ""' "$SAIL_FILE" | grep -oP 'sail-\K[0-9.]+' | head -1 || echo "")
fi
PHP_VERSION="${PHP_VERSION:-$DEFAULT_PHP_VERSION}"

# Extract services from service names
SERVICES=()
KNOWN_SERVICES="mysql pgsql mariadb redis meilisearch memcached mailpit"
for svc in $(yq eval '.services | keys | .[]' "$SAIL_FILE"); do
    [ "$svc" = "laravel.test" ] && continue
    [ "$svc" = "app" ] && continue
    for known in $KNOWN_SERVICES; do
        if [ "$svc" = "$known" ]; then
            SERVICES+=("$svc")
        fi
    done
done

# Extract port overrides
CONFIG_BLOCK=""
for svc in "${SERVICES[@]}"; do
    ports=$(yq eval ".services.${svc}.ports[0]" "$SAIL_FILE" 2>/dev/null || echo "")
    if [ -n "$ports" ] && [ "$ports" != "null" ]; then
        host_port=$(echo "$ports" | grep -oP '^\K[0-9]+(?=:)' || echo "")
        if [ -n "$host_port" ]; then
            CONFIG_BLOCK="${CONFIG_BLOCK}\n  ${svc}:\n    port: ${host_port}"
        fi
    fi
done

echo ""
echo "Detected configuration:"
echo "  PHP version: $PHP_VERSION"
echo "  Services: ${SERVICES[*]}"

# Write frank.yaml
SERVICES_YAML=""
for svc in "${SERVICES[@]}"; do
    SERVICES_YAML="${SERVICES_YAML}  - ${svc}\n"
done

cat > "$PROJECT_DIR/frank.yaml" << EOF
version: 1

php:
  version: "$PHP_VERSION"
  runtime: "frankenphp"

laravel:
  version: "latest"

services:
$(echo -e "$SERVICES_YAML")
EOF

if [ -n "$CONFIG_BLOCK" ]; then
    cat >> "$PROJECT_DIR/frank.yaml" << EOF

config:$(echo -e "$CONFIG_BLOCK")
EOF
fi

echo ""
echo "✅ frank.yaml written from Sail config."
echo ""
echo "⚠️  Note: Custom Sail Dockerfile modifications were not imported."
echo "   Review frank.yaml and run 'just generate' to produce Docker files."

# Run generate
"$SCRIPT_DIR/generate.sh"
