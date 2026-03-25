#!/usr/bin/env bash
# frank/scripts/lib/config.sh — shared functions for Frank scripts
set -euo pipefail

FRANK_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TEMPLATES_DIR="$FRANK_ROOT/templates"
SCRIPTS_DIR="$FRANK_ROOT/scripts"

# Resolve project directory (where frank.yaml lives)
PROJECT_DIR="${FRANK_PROJECT_DIR:-$(pwd)}"
FRANK_YAML="$PROJECT_DIR/frank.yaml"

# Default values matching spec
DEFAULT_PHP_VERSION="8.5"
DEFAULT_PHP_RUNTIME="frankenphp"
DEFAULT_LARAVEL_VERSION="latest"
DEFAULT_SERVICES='["pgsql", "mailpit"]'

# LTS mapping — update when new LTS is released
LARAVEL_LTS_VERSION="11.*"

# Read a value from frank.yaml with a default fallback
# Usage: frank_config_get ".php.version" "8.5"
frank_config_get() {
    local path="$1"
    local default="${2:-}"
    local value
    value=$(yq eval "$path // \"\"" "$FRANK_YAML" 2>/dev/null)
    if [ -z "$value" ] || [ "$value" = "null" ]; then
        echo "$default"
    else
        echo "$value"
    fi
}

# Read services list as space-separated string
frank_services() {
    local services
    services=$(yq eval '.services[]' "$FRANK_YAML" 2>/dev/null || echo "")
    if [ -z "$services" ]; then
        echo "pgsql mailpit"
    else
        echo "$services" | tr '\n' ' '
    fi
}

# Get project name (directory name)
frank_project_name() {
    basename "$PROJECT_DIR"
}

# Check if frank.yaml exists
frank_yaml_exists() {
    [ -f "$FRANK_YAML" ]
}

# Resolve Laravel version string for Composer
frank_resolve_laravel_version() {
    local version
    version=$(frank_config_get ".laravel.version" "latest")
    case "$version" in
        latest) echo "" ;;
        lts)    echo "$LARAVEL_LTS_VERSION" ;;
        *)      echo "$version" ;;
    esac
}
