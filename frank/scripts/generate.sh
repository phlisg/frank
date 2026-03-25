#!/usr/bin/env bash
# frank/scripts/generate.sh — generate Docker files from frank.yaml
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/config.sh"
source "$SCRIPT_DIR/lib/validate.sh"
source "$SCRIPT_DIR/lib/interpolate.sh"

# Generate activate script based on selected services
frank_generate_activate() {
    local output_file="$1"
    local service_aliases=""
    local service_unaliases=""
    local service_alias_names=""

    for service in $(frank_services); do
        case "$service" in
            pgsql)
                service_aliases="${service_aliases}alias psql='docker compose exec db psql -U \${DB_USERNAME:-root} -d \${DB_DATABASE}'\n"
                service_unaliases="${service_unaliases}  unalias psql 2>/dev/null || true\n"
                service_alias_names="${service_alias_names} psql"
                ;;
            mysql|mariadb)
                service_aliases="${service_aliases}alias mysql='docker compose exec db mysql -u \${DB_USERNAME:-root} -p\${DB_PASSWORD:-root} \${DB_DATABASE}'\n"
                service_unaliases="${service_unaliases}  unalias mysql 2>/dev/null || true\n"
                service_alias_names="${service_alias_names} mysql"
                ;;
            redis)
                service_aliases="${service_aliases}alias redis-cli='docker compose exec redis redis-cli'\n"
                service_unaliases="${service_unaliases}  unalias redis-cli 2>/dev/null || true\n"
                service_alias_names="${service_alias_names} redis-cli"
                ;;
        esac
    done

    # Read template, replace placeholders, write output
    sed \
        -e "s|{{service_aliases}}|$(echo -e "$service_aliases")|g" \
        -e "s|{{service_unaliases}}|$(echo -e "$service_unaliases")|g" \
        -e "s|{{service_alias_names}}|$service_alias_names|g" \
        "$TEMPLATES_DIR/activate.tmpl" > "$output_file"

    chmod +x "$output_file"
}

# Parse arguments
OUTPUT_DIR="$PROJECT_DIR"
while [[ $# -gt 0 ]]; do
    case "$1" in
        -f) OUTPUT_DIR="$2"; shift 2 ;;
        *)  echo "Unknown argument: $1" >&2; exit 1 ;;
    esac
done

# Validate
frank_validate

# Read config
RUNTIME=$(frank_config_get '.php.runtime' "$DEFAULT_PHP_RUNTIME")
SERVICES=$(frank_services)

echo "Generating Frank environment..."
echo "  Runtime: $RUNTIME"
echo "  Services: $SERVICES"

# Ensure output directories exist
mkdir -p "$OUTPUT_DIR/.frank/scripts"

# --- Step 1: Generate Dockerfile ---
frank_interpolate \
    "$TEMPLATES_DIR/runtimes/$RUNTIME/Dockerfile.tmpl" \
    "$OUTPUT_DIR/Dockerfile"
echo "  ✓ Dockerfile"

# --- Step 2: Generate server config ---
if [ "$RUNTIME" = "frankenphp" ]; then
    frank_interpolate \
        "$TEMPLATES_DIR/runtimes/$RUNTIME/Caddyfile.tmpl" \
        "$OUTPUT_DIR/Caddyfile"
    rm -f "$OUTPUT_DIR/nginx.Dockerfile" "$OUTPUT_DIR/nginx.conf"
    echo "  ✓ Caddyfile"
elif [ "$RUNTIME" = "fpm" ]; then
    frank_interpolate \
        "$TEMPLATES_DIR/runtimes/$RUNTIME/nginx.conf.tmpl" \
        "$OUTPUT_DIR/nginx.conf"
    frank_interpolate \
        "$TEMPLATES_DIR/runtimes/$RUNTIME/nginx.Dockerfile.tmpl" \
        "$OUTPUT_DIR/nginx.Dockerfile"
    rm -f "$OUTPUT_DIR/Caddyfile"
    echo "  ✓ nginx.conf + nginx.Dockerfile"
fi

# --- Step 3: Generate compose.yaml ---
cp "$TEMPLATES_DIR/base/compose.yaml" "$OUTPUT_DIR/compose.yaml"

# Merge runtime compose fragment (app service + optional nginx)
runtime_compose_tmp=$(mktemp)
frank_interpolate "$TEMPLATES_DIR/runtimes/$RUNTIME/compose.yaml" "$runtime_compose_tmp"

# Add depends_on for database services
for service in $SERVICES; do
    meta_file="$TEMPLATES_DIR/services/$service/meta.yaml"
    if [ -f "$meta_file" ]; then
        category=$(yq eval '.category' "$meta_file")
        if [ "$category" = "database" ] && [ "$service" != "sqlite" ]; then
            yq eval -i '.app.depends_on.db.condition = "service_healthy"' "$runtime_compose_tmp"
        fi
    fi
done

yq eval -i ".services *= load(\"$runtime_compose_tmp\")" "$OUTPUT_DIR/compose.yaml"
rm -f "$runtime_compose_tmp"

# Merge each service's compose fragment and collect volumes
for service in $SERVICES; do
    compose_frag="$TEMPLATES_DIR/services/$service/compose.yaml"
    if [ -f "$compose_frag" ]; then
        interpolated_frag=$(mktemp)
        frank_interpolate "$compose_frag" "$interpolated_frag"
        yq eval -i ".services *= load(\"$interpolated_frag\")" "$OUTPUT_DIR/compose.yaml"

        # Extract named volumes (lines like "- volume_name:/path")
        grep -oP '^\s+- \K[a-z_]+(?=:/)' "$interpolated_frag" | while read -r vol; do
            yq eval -i ".volumes.${vol} = {}" "$OUTPUT_DIR/compose.yaml"
        done

        rm -f "$interpolated_frag"
    fi
done

echo "  ✓ compose.yaml"

# --- Step 4: Generate .env content ---
# Use yq to parse env.yaml reliably (handles colons in values, quotes, etc.)
rm -f "$OUTPUT_DIR/.frank/env.generated.tmp"
for service in $SERVICES; do
    env_file="$TEMPLATES_DIR/services/$service/env.yaml"
    if [ -f "$env_file" ]; then
        # yq outputs key=value pairs, then interpolate {{...}} placeholders
        yq eval 'to_entries | .[] | .key + "=" + (.value | tostring)' "$env_file" \
            | frank_interpolate_string \
            >> "$OUTPUT_DIR/.frank/env.generated.tmp"
    fi
done

if [ -f "$OUTPUT_DIR/.frank/env.generated.tmp" ]; then
    mv "$OUTPUT_DIR/.frank/env.generated.tmp" "$OUTPUT_DIR/.frank/env.generated"
fi
echo "  ✓ .env entries prepared"

# --- Step 5: Generate activate script ---
frank_generate_activate "$OUTPUT_DIR/.frank/scripts/activate"

# Copy shell-setup and psysh config from Frank source
cp "$FRANK_ROOT/scripts/shell-setup" "$OUTPUT_DIR/.frank/scripts/shell-setup" 2>/dev/null || true
cp "$FRANK_ROOT/scripts/.psysh.php" "$OUTPUT_DIR/.frank/.psysh.php" 2>/dev/null || true

echo "  ✓ .frank/ runtime files"

# --- Step 6: Generate justfile (if template exists) ---
if [ -f "$FRANK_ROOT/justfile.tmpl" ]; then
    frank_interpolate "$FRANK_ROOT/justfile.tmpl" "$OUTPUT_DIR/justfile"
    echo "  ✓ justfile"
fi

echo ""
echo "Generation complete!"
