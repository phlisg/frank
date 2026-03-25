#!/usr/bin/env bash
# frank/scripts/lib/validate.sh — validate frank.yaml configuration
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/config.sh"

VALID_SERVICES="pgsql mysql mariadb sqlite redis meilisearch memcached mailpit"
VALID_RUNTIMES="frankenphp fpm"
VALID_PHP_VERSIONS="8.2 8.3 8.4 8.5"

# Validate frank.yaml — exits with error on invalid config
frank_validate() {
    local errors=()

    if ! frank_yaml_exists; then
        echo "Error: frank.yaml not found in $(pwd)" >&2
        exit 1
    fi

    # Validate PHP version
    local php_version
    php_version=$(frank_config_get '.php.version' "$DEFAULT_PHP_VERSION")
    if ! echo "$VALID_PHP_VERSIONS" | grep -qw "$php_version"; then
        errors+=("Invalid PHP version: $php_version (valid: $VALID_PHP_VERSIONS)")
    fi

    # Validate runtime
    local runtime
    runtime=$(frank_config_get '.php.runtime' "$DEFAULT_PHP_RUNTIME")
    if ! echo "$VALID_RUNTIMES" | grep -qw "$runtime"; then
        errors+=("Invalid runtime: $runtime (valid: $VALID_RUNTIMES)")
    fi

    # Validate services
    local services
    services=$(frank_services)
    local db_count=0
    local db_services=""
    local all_ports=()

    for service in $services; do
        # Check service is known
        if ! echo "$VALID_SERVICES" | grep -qw "$service"; then
            errors+=("Unknown service: $service (valid: $VALID_SERVICES)")
            continue
        fi

        # Check for database conflicts
        local meta_file="$TEMPLATES_DIR/services/$service/meta.yaml"
        if [ -f "$meta_file" ]; then
            local category
            category=$(yq eval '.category' "$meta_file")
            if [ "$category" = "database" ] && [ "$service" != "sqlite" ]; then
                db_count=$((db_count + 1))
                db_services="$db_services $service"
            fi

            # Collect ports for uniqueness check
            local default_port
            default_port=$(yq eval '.default_port' "$meta_file")
            if [ "$default_port" != "null" ]; then
                # Check for user override
                local override_port
                override_port=$(frank_config_get ".config.${service}.port" "$default_port")
                all_ports+=("${service}:${override_port}")
            fi
        fi
    done

    # Check only one DB container
    if [ "$db_count" -gt 1 ]; then
        errors+=("Multiple database containers selected:$db_services (pick one)")
    fi

    # Check port uniqueness
    local seen_ports=()
    for entry in "${all_ports[@]:-}"; do
        local svc="${entry%%:*}"
        local port="${entry##*:}"
        for seen in "${seen_ports[@]:-}"; do
            local seen_port="${seen##*:}"
            local seen_svc="${seen%%:*}"
            if [ "$port" = "$seen_port" ] && [ "$svc" != "$seen_svc" ]; then
                errors+=("Port conflict: $svc and $seen_svc both use port $port")
            fi
        done
        seen_ports+=("$entry")
    done

    # Report errors
    if [ ${#errors[@]} -gt 0 ]; then
        echo "Validation errors in frank.yaml:" >&2
        for error in "${errors[@]}"; do
            echo "  - $error" >&2
        done
        exit 1
    fi

    echo "frank.yaml is valid."
}
