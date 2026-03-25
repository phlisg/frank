#!/usr/bin/env bash
# frank/scripts/remove.sh — remove a service from frank.yaml and regenerate
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/config.sh"

SERVICE="${1:-}"
if [ -z "$SERVICE" ]; then
    echo "Usage: frank remove <service>" >&2
    exit 1
fi

if ! frank_yaml_exists; then
    echo "Error: frank.yaml not found." >&2
    exit 1
fi

# Check if service exists
if ! frank_services | grep -qw "$SERVICE"; then
    echo "Service '$SERVICE' is not in frank.yaml." >&2
    exit 1
fi

# Remove service from frank.yaml
yq eval -i "del(.services[] | select(. == \"$SERVICE\"))" "$FRANK_YAML"
# Also remove config for this service if present
yq eval -i "del(.config.$SERVICE)" "$FRANK_YAML"
echo "✅ Removed '$SERVICE' from frank.yaml"

# Regenerate
"$SCRIPT_DIR/generate.sh"
