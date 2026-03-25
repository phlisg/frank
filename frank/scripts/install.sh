#!/usr/bin/env bash
# frank/scripts/install.sh — install Laravel using frank.yaml config
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/config.sh"

if [ -f "$PROJECT_DIR/artisan" ]; then
    echo "❌ Laravel already installed. Run 'just reset' to reinstall." >&2
    exit 1
fi

if ! frank_yaml_exists; then
    echo "Error: frank.yaml not found. Run 'just init' first." >&2
    exit 1
fi

# Resolve Laravel version
LARAVEL_VERSION=$(frank_resolve_laravel_version)
VERSION_ARG=""
if [ -n "$LARAVEL_VERSION" ]; then
    VERSION_ARG="$LARAVEL_VERSION"
fi

PROJECT_NAME=$(frank_project_name)

echo "⏳ Installing Laravel..."
echo "  Version: ${LARAVEL_VERSION:-latest}"
echo "  Project: $PROJECT_NAME"

# Backup Frank files
mv "$PROJECT_DIR/README.md" "$PROJECT_DIR/README.frank.md" 2>/dev/null || true
mv "$PROJECT_DIR/.gitignore" "$PROJECT_DIR/.gitignore.frank" 2>/dev/null || true

# Run composer in a disposable container
docker run --rm \
    -v "$PROJECT_DIR:/app" \
    -w /app \
    -u "$(id -u):$(id -g)" \
    composer:latest \
    sh -c "composer create-project --prefer-dist laravel/laravel .temp-laravel $VERSION_ARG && \
           cp -r .temp-laravel/* . && \
           cp .temp-laravel/.env.example . 2>/dev/null || true && \
           cp .env.example .env 2>/dev/null || true && \
           cp .temp-laravel/.gitignore . 2>/dev/null || true && \
           rm -rf .temp-laravel && \
           php artisan key:generate"

# Apply generated env entries
if [ -f "$PROJECT_DIR/.frank/env.generated" ]; then
    while IFS='=' read -r key value; do
        [ -z "$key" ] && continue
        # Replace or append in .env
        if grep -q "^${key}=" "$PROJECT_DIR/.env" 2>/dev/null; then
            sed -i "s|^${key}=.*|${key}=${value}|" "$PROJECT_DIR/.env"
        else
            echo "${key}=${value}" >> "$PROJECT_DIR/.env"
        fi
    done < "$PROJECT_DIR/.frank/env.generated"
fi

# Update vite.config.js for HMR
if [ -f "$PROJECT_DIR/vite.config.js" ] && ! grep -q "server:" "$PROJECT_DIR/vite.config.js"; then
    sed -i "/defineConfig({/a \    server: {\n        host: '0.0.0.0',\n        port: 5173,\n        hmr: {\n            host: 'localhost',\n        },\n    }," "$PROJECT_DIR/vite.config.js"
fi

# Copy psysh config
cp "$PROJECT_DIR/.frank/.psysh.php" "$PROJECT_DIR/.psysh.php" 2>/dev/null || true

# Restore Frank's README and merge gitignore
mv "$PROJECT_DIR/README.frank.md" "$PROJECT_DIR/README.md" 2>/dev/null || true
if [ -f "$PROJECT_DIR/.gitignore.frank" ]; then
    cat "$PROJECT_DIR/.gitignore.frank" >> "$PROJECT_DIR/.gitignore"
    rm "$PROJECT_DIR/.gitignore.frank"
fi

echo ""
echo "🎈 Laravel installed!"
echo "🚀 Run 'just up' to start the development environment."
