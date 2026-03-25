#!/usr/bin/env bash
# frank/scripts/lib/interpolate.sh — resolve {{...}} template variables
set -euo pipefail

# Source config library if not already loaded
if [ -z "${FRANK_ROOT:-}" ]; then
    source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/config.sh"
fi

# Build a sed expression file from frank.yaml for template interpolation
# Resolves: {{php.version}}, {{project_name}}, {{config.SERVICE.KEY:-DEFAULT}}
frank_build_interpolation_sed() {
    local sed_file="$1"
    local project_name
    project_name=$(frank_project_name)

    > "$sed_file"

    # Static variables
    echo "s|{{project_name}}|${project_name}|g" >> "$sed_file"
    echo "s|{{php.version}}|$(frank_config_get '.php.version' "$DEFAULT_PHP_VERSION")|g" >> "$sed_file"
    echo "s|{{php.runtime}}|$(frank_config_get '.php.runtime' "$DEFAULT_PHP_RUNTIME")|g" >> "$sed_file"

    # Config overrides: {{config.SERVICE.KEY:-DEFAULT}}
    # Parse all services and their config overrides
    for service in $(frank_services); do
        # Read all config keys for this service
        local keys
        keys=$(yq eval ".config.${service} | keys | .[]" "$FRANK_YAML" 2>/dev/null || echo "")
        for key in $keys; do
            local value
            value=$(yq eval ".config.${service}.${key}" "$FRANK_YAML" 2>/dev/null || echo "")
            if [ -n "$value" ] && [ "$value" != "null" ]; then
                # Replace {{config.SERVICE.KEY:-DEFAULT}} with the actual value
                echo "s|{{config.${service}.${key}:-[^}]*}}|${value}|g" >> "$sed_file"
                # Also replace {{config.SERVICE.KEY}} without default
                echo "s|{{config.${service}.${key}}}|${value}|g" >> "$sed_file"
            fi
        done
    done

    # Resolve remaining {{config.X.Y:-DEFAULT}} to their defaults
    echo 's|{{config\.[^}]*:-\([^}]*\)}}|\1|g' >> "$sed_file"
    # Remove any remaining unresolved {{config.X.Y}} (no default)
    echo 's|{{config\.[^}]*}}||g' >> "$sed_file"
}

# Interpolate a template file and write to output
# Usage: frank_interpolate input_file output_file
frank_interpolate() {
    local input="$1"
    local output="$2"
    local sed_file
    sed_file=$(mktemp)
    trap "rm -f '$sed_file'" RETURN

    frank_build_interpolation_sed "$sed_file"
    sed -f "$sed_file" "$input" > "$output"
}

# Interpolate a string (from stdin or argument)
# Usage: echo "{{php.version}}" | frank_interpolate_string
frank_interpolate_string() {
    local sed_file
    sed_file=$(mktemp)
    trap "rm -f '$sed_file'" RETURN

    frank_build_interpolation_sed "$sed_file"
    sed -f "$sed_file"
}
