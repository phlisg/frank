#!/usr/bin/env bash
# frank/scripts/add.sh — add a service to frank.yaml and regenerate
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/config.sh"

SERVICE="${1:-}"
if [ -z "$SERVICE" ]; then
    echo "Usage: frank add <service>" >&2
    echo "Available: pgsql mysql mariadb sqlite redis meilisearch memcached mailpit" >&2
    exit 1
fi

if ! frank_yaml_exists; then
    echo "Error: frank.yaml not found. Run 'just init' first." >&2
    exit 1
fi

# Check if service already exists
if frank_services | grep -qw "$SERVICE"; then
    echo "Service '$SERVICE' is already in frank.yaml." >&2
    exit 1
fi

# Add service to frank.yaml
yq eval -i ".services += [\"$SERVICE\"]" "$FRANK_YAML"
echo "✅ Added '$SERVICE' to frank.yaml"

# Regenerate
"$SCRIPT_DIR/generate.sh"
