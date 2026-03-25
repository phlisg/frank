# Frank Flexibility Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers-extended-cc:subagent-driven-development (if subagents available) or superpowers-extended-cc:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Transform Frank from a hardcoded Docker-based Laravel dev environment into a flexible, config-driven tool with selectable services, PHP runtimes, and Sail interop.

**Architecture:** Template-based generation. `frank.yaml` is the config source of truth. Shell scripts read it, process templates via `yq`, and output `compose.yaml`, `Dockerfile`, and server config. The justfile is a thin wrapper delegating to these scripts.

**Tech Stack:** Shell (bash), yq (YAML processing), Just (task runner), Docker Compose v2

**Spec:** `docs/superpowers/specs/2026-03-25-frank-flexibility-design.md`

---

## File Structure

### New files to create

```
frank/
  templates/
    base/
      compose.yaml                    # base compose structure (networks, volumes)
    services/
      pgsql/
        compose.yaml                  # compose fragment
        env.yaml                      # .env variables
        meta.yaml                     # metadata for validation/wizard
      mysql/
        compose.yaml
        env.yaml
        meta.yaml
      mariadb/
        compose.yaml
        env.yaml
        meta.yaml
      sqlite/
        env.yaml
        meta.yaml
      redis/
        compose.yaml
        env.yaml
        meta.yaml
      meilisearch/
        compose.yaml
        env.yaml
        meta.yaml
      memcached/
        compose.yaml
        env.yaml
        meta.yaml
      mailpit/
        compose.yaml
        env.yaml
        meta.yaml
    runtimes/
      frankenphp/
        Dockerfile.tmpl
        Caddyfile.tmpl
        compose.yaml                  # app service definition
      fpm/
        Dockerfile.tmpl
        nginx.Dockerfile.tmpl
        nginx.conf.tmpl
        compose.yaml                  # app + nginx service definitions
    activate.tmpl                     # shell alias template
  scripts/
    generate.sh                       # core generator engine
    init.sh                           # interactive wizard
    install.sh                        # Laravel project creation (replaces laravel-init.sh)
    add.sh                            # add service to frank.yaml
    remove.sh                         # remove service from frank.yaml
    sail-import.sh                    # Sail → frank.yaml
    sail-export.sh                    # frank.yaml → Sail
    lib/
      config.sh                       # frank.yaml parser (shared functions)
      validate.sh                     # validation functions
      interpolate.sh                  # {{...}} template interpolation
  justfile.tmpl                       # template for generated justfile
```

### Files to modify
- `justfile` — becomes the generated wrapper (or the tool's own justfile that generates it)
- `.gitignore` — add generated file patterns

### Files to remove (replaced by generation)
- `docker-compose.yml` → replaced by generated `compose.yaml`
- `Dockerfile` → replaced by generated Dockerfile from runtime template
- `Caddyfile` → replaced by generated Caddyfile from runtime template
- `frank/scripts/activate` → replaced by generated activate from template
- `frank/scripts/laravel-init.sh` → replaced by `frank/scripts/install.sh`
- `frank/scripts/shell-setup` → moved to `.frank/scripts/shell-setup` (generated)

---

## Task 0: Install yq dependency and set up project scaffold

**Files:**
- Create: `frank/scripts/lib/config.sh`
- Modify: `.gitignore`

This foundational task ensures the build environment is ready and shared library functions exist.

- [ ] **Step 1: Verify yq is available**

```bash
# Install yq if not present
# On Fedora:
sudo dnf install -y yq
# Or via Go:
go install github.com/mikefarah/yq/v4@latest
# Verify:
yq --version
```

- [ ] **Step 2: Create the shared config library**

Create `frank/scripts/lib/config.sh` — this is the shared function library that all scripts source.

```bash
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
# Usage: frank_services
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
# "latest" → "", "lts" → "$LARAVEL_LTS_VERSION", other → passthrough
frank_resolve_laravel_version() {
    local version
    version=$(frank_config_get ".laravel.version" "latest")
    case "$version" in
        latest) echo "" ;;
        lts)    echo "$LARAVEL_LTS_VERSION" ;;
        *)      echo "$version" ;;
    esac
}
```

- [ ] **Step 3: Create template directories**

```bash
mkdir -p frank/templates/{base,services/{pgsql,mysql,mariadb,sqlite,redis,meilisearch,memcached,mailpit},runtimes/{frankenphp,fpm}}
mkdir -p frank/scripts/lib
```

- [ ] **Step 4: Update .gitignore**

Append patterns for generated output in target projects (not this repo):

```
# Already tracked in .gitignore:
# .history, .vscode, .idea, .todos/, .sidecar*
```

No changes needed to .gitignore in the tool repo itself — all template files are tracked.

- [ ] **Step 5: Commit scaffold**

```bash
git add frank/scripts/lib/config.sh frank/templates/
git commit -m "feat: add project scaffold and shared config library for flexible Frank"
```

---

## Task 1: Create service templates (all 8 services)

**Files:**
- Create: `frank/templates/services/{pgsql,mysql,mariadb,sqlite,redis,meilisearch,memcached,mailpit}/{meta.yaml,env.yaml,compose.yaml}`

Each service gets three files: `meta.yaml` (validation/wizard), `env.yaml` (.env injection), `compose.yaml` (compose fragment). SQLite has no `compose.yaml`.

- [ ] **Step 1: Create pgsql templates**

`frank/templates/services/pgsql/meta.yaml`:
```yaml
name: PostgreSQL
category: database
default_port: 5432
conflicts: [mysql, mariadb]
env_prefix: DB_
image: postgres
container_name: db
```

`frank/templates/services/pgsql/env.yaml`:
```yaml
DB_CONNECTION: pgsql
DB_HOST: db
DB_PORT: "{{config.pgsql.port:-5432}}"
DB_DATABASE: "{{project_name}}"
DB_USERNAME: root
DB_PASSWORD: root
```

`frank/templates/services/pgsql/compose.yaml`:
```yaml
db:
  image: "postgres:{{config.pgsql.version:-latest}}"
  environment:
    POSTGRES_DB: "${DB_DATABASE}"
    POSTGRES_USER: "${DB_USERNAME}"
    POSTGRES_PASSWORD: "${DB_PASSWORD}"
  volumes:
    - db_data:/var/lib/postgresql/data
  ports:
    - "{{config.pgsql.port:-5432}}:5432"
  networks:
    - frank
  healthcheck:
    test: ["CMD-SHELL", "pg_isready -U ${DB_USERNAME:-root}"]
    interval: 5s
    timeout: 3s
    retries: 10
```

- [ ] **Step 2: Create mysql templates**

`frank/templates/services/mysql/meta.yaml`:
```yaml
name: MySQL
category: database
default_port: 3306
conflicts: [pgsql, mariadb]
env_prefix: DB_
image: mysql
container_name: db
```

`frank/templates/services/mysql/env.yaml`:
```yaml
DB_CONNECTION: mysql
DB_HOST: db
DB_PORT: "{{config.mysql.port:-3306}}"
DB_DATABASE: "{{project_name}}"
DB_USERNAME: root
DB_PASSWORD: root
```

`frank/templates/services/mysql/compose.yaml`:
```yaml
db:
  image: "mysql:{{config.mysql.version:-latest}}"
  environment:
    MYSQL_ROOT_PASSWORD: "${DB_PASSWORD}"
    MYSQL_DATABASE: "${DB_DATABASE}"
    MYSQL_USER: "${DB_USERNAME}"
    MYSQL_PASSWORD: "${DB_PASSWORD}"
    MYSQL_ALLOW_EMPTY_PASSWORD: "yes"
  volumes:
    - db_data:/var/lib/mysql
  ports:
    - "{{config.mysql.port:-3306}}:3306"
  networks:
    - frank
  healthcheck:
    test: ["CMD", "mysqladmin", "ping", "-h", "localhost", "-p${DB_PASSWORD}"]
    interval: 5s
    timeout: 3s
    retries: 10
```

- [ ] **Step 3: Create mariadb templates**

`frank/templates/services/mariadb/meta.yaml`:
```yaml
name: MariaDB
category: database
default_port: 3306
conflicts: [pgsql, mysql]
env_prefix: DB_
image: mariadb
container_name: db
```

`frank/templates/services/mariadb/env.yaml`:
```yaml
DB_CONNECTION: mariadb
DB_HOST: db
DB_PORT: "{{config.mariadb.port:-3306}}"
DB_DATABASE: "{{project_name}}"
DB_USERNAME: root
DB_PASSWORD: root
```

`frank/templates/services/mariadb/compose.yaml`:
```yaml
db:
  image: "mariadb:{{config.mariadb.version:-latest}}"
  environment:
    MARIADB_ROOT_PASSWORD: "${DB_PASSWORD}"
    MARIADB_DATABASE: "${DB_DATABASE}"
    MARIADB_USER: "${DB_USERNAME}"
    MARIADB_PASSWORD: "${DB_PASSWORD}"
    MARIADB_ALLOW_EMPTY_ROOT_PASSWORD: "yes"
  volumes:
    - db_data:/var/lib/mysql
  ports:
    - "{{config.mariadb.port:-3306}}:3306"
  networks:
    - frank
  healthcheck:
    test: ["CMD", "healthcheck.sh", "--connect", "--innodb_initialized"]
    interval: 5s
    timeout: 3s
    retries: 10
```

- [ ] **Step 4: Create sqlite templates**

`frank/templates/services/sqlite/meta.yaml`:
```yaml
name: SQLite
category: database
default_port: null
conflicts: []
env_prefix: DB_
image: null
container_name: null
```

`frank/templates/services/sqlite/env.yaml`:
```yaml
DB_CONNECTION: sqlite
# No DB_HOST, DB_PORT, DB_USERNAME, DB_PASSWORD needed
```

No `compose.yaml` — SQLite has no container.

- [ ] **Step 5: Create redis templates**

`frank/templates/services/redis/meta.yaml`:
```yaml
name: Redis
category: cache
default_port: 6379
conflicts: []
env_prefix: REDIS_
image: redis
container_name: redis
```

`frank/templates/services/redis/env.yaml`:
```yaml
REDIS_HOST: redis
REDIS_PORT: "{{config.redis.port:-6379}}"
REDIS_PASSWORD: ""
CACHE_STORE: redis
SESSION_DRIVER: redis
QUEUE_CONNECTION: redis
```

`frank/templates/services/redis/compose.yaml`:
```yaml
redis:
  image: "redis:{{config.redis.version:-alpine}}"
  ports:
    - "{{config.redis.port:-6379}}:6379"
  networks:
    - frank
  volumes:
    - redis_data:/data
  healthcheck:
    test: ["CMD", "redis-cli", "ping"]
    interval: 5s
    timeout: 3s
    retries: 10
```

- [ ] **Step 6: Create meilisearch templates**

`frank/templates/services/meilisearch/meta.yaml`:
```yaml
name: Meilisearch
category: search
default_port: 7700
conflicts: []
env_prefix: MEILISEARCH_
image: getmeili/meilisearch
container_name: meilisearch
```

`frank/templates/services/meilisearch/env.yaml`:
```yaml
MEILISEARCH_HOST: "http://meilisearch:{{config.meilisearch.port:-7700}}"
MEILISEARCH_NO_ANALYTICS: "true"
SCOUT_DRIVER: meilisearch
```

`frank/templates/services/meilisearch/compose.yaml`:
```yaml
meilisearch:
  image: "getmeili/meilisearch:{{config.meilisearch.version:-latest}}"
  ports:
    - "{{config.meilisearch.port:-7700}}:7700"
  networks:
    - frank
  volumes:
    - meilisearch_data:/meili_data
  environment:
    MEILI_NO_ANALYTICS: "true"
  healthcheck:
    test: ["CMD", "wget", "--no-verbose", "--spider", "http://127.0.0.1:7700/health"]
    interval: 5s
    timeout: 3s
    retries: 10
```

- [ ] **Step 7: Create memcached templates**

`frank/templates/services/memcached/meta.yaml`:
```yaml
name: Memcached
category: cache
default_port: 11211
conflicts: []
env_prefix: MEMCACHED_
image: memcached
container_name: memcached
```

`frank/templates/services/memcached/env.yaml`:
```yaml
MEMCACHED_HOST: memcached
CACHE_STORE: memcached
```

`frank/templates/services/memcached/compose.yaml`:
```yaml
memcached:
  image: "memcached:{{config.memcached.version:-alpine}}"
  ports:
    - "{{config.memcached.port:-11211}}:11211"
  networks:
    - frank
```

- [ ] **Step 8: Create mailpit templates**

`frank/templates/services/mailpit/meta.yaml`:
```yaml
name: Mailpit
category: mail
default_port: 1025
conflicts: []
env_prefix: MAIL_
image: axllent/mailpit
container_name: mailpit
```

`frank/templates/services/mailpit/env.yaml`:
```yaml
MAIL_MAILER: smtp
MAIL_HOST: mailpit
MAIL_PORT: "{{config.mailpit.port:-1025}}"
MAIL_USERNAME: ""
MAIL_PASSWORD: ""
MAIL_ENCRYPTION: null
MAIL_FROM_ADDRESS: "hello@example.com"
MAIL_FROM_NAME: "{{project_name}}"
```

`frank/templates/services/mailpit/compose.yaml`:
```yaml
mailpit:
  image: "axllent/mailpit:{{config.mailpit.version:-latest}}"
  ports:
    - "{{config.mailpit.port:-1025}}:1025"
    - "{{config.mailpit.dashboard_port:-8025}}:8025"
  networks:
    - frank
```

- [ ] **Step 9: Commit all service templates**

```bash
git add frank/templates/services/
git commit -m "feat: add service templates for all 8 supported services"
```

---

## Task 2: Create runtime templates (FrankenPHP + FPM)

**Files:**
- Create: `frank/templates/runtimes/frankenphp/{Dockerfile.tmpl,Caddyfile.tmpl,compose.yaml}`
- Create: `frank/templates/runtimes/fpm/{Dockerfile.tmpl,nginx.Dockerfile.tmpl,nginx.conf.tmpl,compose.yaml}`
- Create: `frank/templates/base/compose.yaml`

- [ ] **Step 1: Create base compose template**

`frank/templates/base/compose.yaml`:
```yaml
# Generated by Frank — edit frank.yaml instead
services: {}

networks:
  frank:
    driver: bridge

volumes: {}
```

Note: `services: {}` and `volumes: {}` are placeholders — the generator will populate them. The explicit `services:` key is required for yq's deep merge to work correctly.

- [ ] **Step 2: Create FrankenPHP Dockerfile template**

`frank/templates/runtimes/frankenphp/Dockerfile.tmpl`:
```dockerfile
# Generated by Frank — edit frank.yaml instead
FROM dunglas/frankenphp:1-php{{php.version}}

# Install system dependencies for Laravel
RUN apt-get update && apt-get install -y \
    git \
    curl \
    nodejs \
    npm \
    libpng-dev \
    libonig-dev \
    libxml2-dev \
    libicu-dev \
    libzip-dev \
    libpq-dev \
    postgresql-client \
    default-mysql-client \
    zip \
    unzip \
    && docker-php-ext-install pdo_mysql pdo_pgsql pgsql mbstring exif pcntl bcmath gd intl zip \
    && npm install -g npm pnpm bun \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# Copy the Caddyfile
COPY Caddyfile /etc/caddy/Caddyfile

# Set proper permissions and entrypoint
RUN echo '#!/bin/bash\n\
if [ -d "/app/storage" ]; then\n\
  chown -R www-data:www-data /app/storage /app/bootstrap/cache 2>/dev/null || true\n\
  chmod -R 755 /app/storage /app/bootstrap/cache 2>/dev/null || true\n\
fi\n\
if [ -f "/app/.env.example" ] && [ ! -f "/app/.env" ]; then\n\
  cp /app/.env.example /app/.env\n\
  php artisan key:generate --no-interaction 2>/dev/null || true\n\
fi\n\
if [ ! -d /app/storage/psysh ]; then\n\
  mkdir -p /app/storage/psysh 2>/dev/null || true\n\
  chmod 755 /app/storage/psysh 2>/dev/null || true\n\
fi\n\
npm install --prefix /app 2>/dev/null || true\n\
npm run build --prefix /app 2>/dev/null &\n\
exec "$@"' > /entrypoint.sh && chmod +x /entrypoint.sh

# Install Composer
COPY --from=composer:latest /usr/bin/composer /usr/bin/composer

EXPOSE 8000 5173

ENTRYPOINT ["/entrypoint.sh"]
CMD ["frankenphp", "run", "--config", "/etc/caddy/Caddyfile"]
```

- [ ] **Step 3: Create FrankenPHP Caddyfile template**

`frank/templates/runtimes/frankenphp/Caddyfile.tmpl`:
```
# Generated by Frank — edit frank.yaml instead
{
    frankenphp
    order php_server before file_server
}

:8000 {
    root * /app/public

    php_server

    try_files {path} {path}/ /index.php?{query}

    header {
        -Server
        X-Content-Type-Options nosniff
        X-Frame-Options DENY
        Referrer-Policy strict-origin-when-cross-origin
    }

    @static {
        file
        path *.ico *.css *.js *.gif *.jpg *.jpeg *.png *.svg *.woff *.woff2 *.ttf *.eot
    }
    header @static Cache-Control max-age=31536000

    log {
        output stdout
        format console
    }
}
```

- [ ] **Step 4: Create FrankenPHP compose fragment**

`frank/templates/runtimes/frankenphp/compose.yaml`:
```yaml
app:
  build:
    context: .
  ports:
    - "8000:8000"
    - "5173:5173"
    - "2019:2019"
  networks:
    - frank
  volumes:
    - .:/app
  environment:
    - APP_ENV=local
    - APP_DEBUG=true
    - SERVER_NAME=:8000
    - PSYSH_CONFIG=/app/.psysh.php
    - PSYSH_HOME=/app/storage/psysh
  env_file:
    - .env
  working_dir: /app
  tty: true
  user: "${UID:-1000}:${GID:-1000}"
  stdin_open: true
```

- [ ] **Step 5: Create FPM Dockerfile template**

`frank/templates/runtimes/fpm/Dockerfile.tmpl`:
```dockerfile
# Generated by Frank — edit frank.yaml instead
FROM php:{{php.version}}-fpm

# Install system dependencies for Laravel
RUN apt-get update && apt-get install -y \
    git \
    curl \
    nodejs \
    npm \
    libpng-dev \
    libonig-dev \
    libxml2-dev \
    libicu-dev \
    libzip-dev \
    libpq-dev \
    postgresql-client \
    default-mysql-client \
    zip \
    unzip \
    && docker-php-ext-install pdo_mysql pdo_pgsql pgsql mbstring exif pcntl bcmath gd intl zip \
    && npm install -g npm pnpm bun \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# Set proper permissions and entrypoint
RUN echo '#!/bin/bash\n\
if [ -d "/app/storage" ]; then\n\
  chown -R www-data:www-data /app/storage /app/bootstrap/cache 2>/dev/null || true\n\
  chmod -R 755 /app/storage /app/bootstrap/cache 2>/dev/null || true\n\
fi\n\
if [ -f "/app/.env.example" ] && [ ! -f "/app/.env" ]; then\n\
  cp /app/.env.example /app/.env\n\
  php artisan key:generate --no-interaction 2>/dev/null || true\n\
fi\n\
if [ ! -d /app/storage/psysh ]; then\n\
  mkdir -p /app/storage/psysh 2>/dev/null || true\n\
  chmod 755 /app/storage/psysh 2>/dev/null || true\n\
fi\n\
npm install --prefix /app 2>/dev/null || true\n\
npm run build --prefix /app 2>/dev/null &\n\
exec "$@"' > /entrypoint.sh && chmod +x /entrypoint.sh

# Install Composer
COPY --from=composer:latest /usr/bin/composer /usr/bin/composer

WORKDIR /app
EXPOSE 9000

ENTRYPOINT ["/entrypoint.sh"]
CMD ["php-fpm"]
```

- [ ] **Step 6: Create FPM nginx Dockerfile template**

`frank/templates/runtimes/fpm/nginx.Dockerfile.tmpl`:
```dockerfile
# Generated by Frank — edit frank.yaml instead
FROM nginx:alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
```

- [ ] **Step 7: Create FPM nginx.conf template**

`frank/templates/runtimes/fpm/nginx.conf.tmpl`:
```nginx
# Generated by Frank — edit frank.yaml instead
server {
    listen 8000;
    server_name _;
    root /app/public;
    index index.php index.html;

    charset utf-8;

    location / {
        try_files $uri $uri/ /index.php?$query_string;
    }

    location = /favicon.ico { access_log off; log_not_found off; }
    location = /robots.txt  { access_log off; log_not_found off; }

    error_page 404 /index.php;

    location ~ \.php$ {
        fastcgi_pass app:9000;
        fastcgi_param SCRIPT_FILENAME $realpath_root$fastcgi_script_name;
        include fastcgi_params;
    }

    location ~ /\.(?!well-known).* {
        deny all;
    }
}
```

- [ ] **Step 8: Create FPM compose fragment**

`frank/templates/runtimes/fpm/compose.yaml`:
```yaml
app:
  build:
    context: .
    dockerfile: Dockerfile
  networks:
    - frank
  volumes:
    - .:/app
  environment:
    - APP_ENV=local
    - APP_DEBUG=true
    - PSYSH_CONFIG=/app/.psysh.php
    - PSYSH_HOME=/app/storage/psysh
  env_file:
    - .env
  working_dir: /app
  tty: true
  user: "${UID:-1000}:${GID:-1000}"
  stdin_open: true

nginx:
  build:
    context: .
    dockerfile: nginx.Dockerfile
  ports:
    - "8000:8000"
    - "5173:5173"
  networks:
    - frank
  volumes:
    - .:/app
  depends_on:
    - app
```

- [ ] **Step 9: Commit runtime templates**

```bash
git add frank/templates/runtimes/ frank/templates/base/
git commit -m "feat: add FrankenPHP and FPM runtime templates with base compose"
```

---

## Task 3: Create the template interpolation engine

**Files:**
- Create: `frank/scripts/lib/interpolate.sh`

This is the core engine that resolves `{{...}}` placeholders in templates.

- [ ] **Step 1: Write the interpolation library**

`frank/scripts/lib/interpolate.sh`:
```bash
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
                # This sed pattern matches the full {{config.X.Y:-Z}} and replaces with the value
                echo "s|{{config.${service}.${key}:-[^}]*}}|${value}|g" >> "$sed_file"
                # Also replace {{config.SERVICE.KEY}} without default
                echo "s|{{config.${service}.${key}}}|${value}|g" >> "$sed_file"
            fi
        done
    done

    # Resolve remaining {{config.X.Y:-DEFAULT}} to their defaults
    # This catches any config references where no override was set
    # Uses a perl-compatible approach for the default extraction
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
```

- [ ] **Step 2: Test interpolation manually**

Create a temporary `frank.yaml` and test:

```bash
cat > /tmp/test-frank.yaml << 'EOF'
version: 1
php:
  version: "8.4"
  runtime: frankenphp
services:
  - pgsql
  - mailpit
config:
  pgsql:
    port: 5433
EOF

cd /tmp && mkdir -p test-frank && cd test-frank
cp /tmp/test-frank.yaml frank.yaml
source /home/json/code/php/frank/frank/scripts/lib/config.sh
source /home/json/code/php/frank/frank/scripts/lib/interpolate.sh
echo '{{php.version}} - {{config.pgsql.port:-5432}} - {{project_name}}' | frank_interpolate_string
# Expected: 8.4 - 5433 - test-frank
```

- [ ] **Step 3: Commit interpolation engine**

```bash
git add frank/scripts/lib/interpolate.sh
git commit -m "feat: add template interpolation engine for {{...}} variables"
```

---

## Task 4: Create the validation library

**Files:**
- Create: `frank/scripts/lib/validate.sh`

- [ ] **Step 1: Write the validation library**

`frank/scripts/lib/validate.sh`:
```bash
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
```

- [ ] **Step 2: Test validation**

```bash
# Test with conflicting databases
cat > /tmp/test-frank/frank.yaml << 'EOF'
version: 1
services:
  - pgsql
  - mysql
EOF

cd /tmp/test-frank
source /home/json/code/php/frank/frank/scripts/lib/validate.sh
frank_validate
# Expected: Error about multiple database containers
```

- [ ] **Step 3: Commit validation library**

```bash
git add frank/scripts/lib/validate.sh
git commit -m "feat: add frank.yaml validation (service conflicts, port uniqueness, version checks)"
```

---

## Task 5: Build the generator script (`generate.sh`)

**Files:**
- Create: `frank/scripts/generate.sh`

This is the main orchestrator that reads `frank.yaml`, validates, interpolates templates, merges compose fragments, and writes output files.

- [ ] **Step 1: Write generate.sh**

`frank/scripts/generate.sh`:
```bash
#!/usr/bin/env bash
# frank/scripts/generate.sh — generate Docker files from frank.yaml
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/config.sh"
source "$SCRIPT_DIR/lib/validate.sh"
source "$SCRIPT_DIR/lib/interpolate.sh"

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
ENV_CONTENT=""
for service in $SERVICES; do
    env_file="$TEMPLATES_DIR/services/$service/env.yaml"
    if [ -f "$env_file" ]; then
        # yq outputs key=value pairs, then interpolate {{...}} placeholders
        yq eval 'to_entries | .[] | .key + "=" + (.value | tostring)' "$env_file" \
            | frank_interpolate_string \
            >> "$OUTPUT_DIR/.frank/env.generated.tmp"
    fi
done
echo "  ✓ .env entries prepared"

# Save env content for use by install.sh
mkdir -p "$OUTPUT_DIR/.frank"
if [ -f "$OUTPUT_DIR/.frank/env.generated.tmp" ]; then
    mv "$OUTPUT_DIR/.frank/env.generated.tmp" "$OUTPUT_DIR/.frank/env.generated"
fi

# --- Step 5: Generate activate script ---
mkdir -p "$OUTPUT_DIR/.frank/scripts"
frank_generate_activate "$OUTPUT_DIR/.frank/scripts/activate"

# Copy shell-setup and psysh config from Frank source
cp "$FRANK_ROOT/scripts/shell-setup" "$OUTPUT_DIR/.frank/scripts/shell-setup" 2>/dev/null || true
cp "$FRANK_ROOT/scripts/.psysh.php" "$OUTPUT_DIR/.frank/.psysh.php" 2>/dev/null || true

# --- Step 6: Generate justfile ---
frank_interpolate "$FRANK_ROOT/justfile.tmpl" "$OUTPUT_DIR/justfile"

echo "  ✓ .frank/ runtime files"
echo "  ✓ justfile"
echo ""
echo "Generation complete!"
```

**Key design decisions in generate.sh:**
- No `local` keyword outside functions (bash only allows `local` inside functions)
- Uses `yq eval 'to_entries | ...'` to parse env.yaml instead of fragile `IFS` splitting — this correctly handles values with colons (like URLs)
- Volume extraction uses `grep` on the interpolated (not raw) fragment
- Generates the justfile from `justfile.tmpl` as part of the pipeline
- Uses `$FRANK_ROOT` (set by config.sh) for source file paths — no relative path gymnastics

Note: The `frank_generate_activate` function is defined in the next task (Task 6). generate.sh cannot be fully tested until Task 6 is complete.

- [ ] **Step 2: Make generate.sh executable**

```bash
chmod +x frank/scripts/generate.sh
```

- [ ] **Step 3: Commit generator**

```bash
git add frank/scripts/generate.sh
git commit -m "feat: add core generator script (compose, Dockerfile, server config)"
```

---

## Task 6: Create the activate template and generator

**Files:**
- Create: `frank/templates/activate.tmpl`
- Modify: `frank/scripts/generate.sh` (add `frank_generate_activate` function)

- [ ] **Step 1: Write activate.tmpl**

`frank/templates/activate.tmpl`:
```bash
#!/bin/bash
# Generated by Frank — edit frank.yaml instead

# Check if containers are running
if ! docker compose ps app | grep -q 'Up'; then
  echo '❌ Frank containers are not running. Please run "just up" first.'
  return 1
fi

# Core aliases (always present)
alias composer='docker compose exec app composer'
alias artisan='docker compose exec app php artisan'
alias php='docker compose exec app php'
alias tinker='docker compose exec app php artisan tinker'
alias npm='docker compose exec app npm'
alias bun='docker compose exec app bun'

# Service-specific aliases
{{service_aliases}}

# Environment marker
export FRANK_ENV_ACTIVE=1

# Prompt decoration
if [[ -z "$FRANK_ORIGINAL_PS1" ]]; then
  export FRANK_ORIGINAL_PS1="$PS1"
fi
export PS1="(frank) $FRANK_ORIGINAL_PS1"

# Deactivate function
deactivate() {
  unalias composer artisan php tinker npm bun 2>/dev/null || true
  {{service_unaliases}}
  if [[ -n "$FRANK_ORIGINAL_PS1" ]]; then
    export PS1="$FRANK_ORIGINAL_PS1"
    unset FRANK_ORIGINAL_PS1
  fi
  unset FRANK_ENV_ACTIVE
  unset -f deactivate
  echo '📦 Frank environment deactivated'
}

echo ''
echo '🏕️ Frank environment activated!'
echo '📦 Available commands: composer artisan php tinker npm bun {{service_alias_names}}'
echo '🔧 To deactivate, run: deactivate'
```

- [ ] **Step 2: Add frank_generate_activate function to generate.sh**

Add this function to `frank/scripts/generate.sh` (or better, to a shared lib):

```bash
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
```

- [ ] **Step 3: Commit activate generation**

```bash
git add frank/templates/activate.tmpl frank/scripts/generate.sh
git commit -m "feat: add dynamic activate script generation based on selected services"
```

---

## Task 7: Create the init wizard (`init.sh`)

**Files:**
- Create: `frank/scripts/init.sh`

- [ ] **Step 1: Write init.sh**

`frank/scripts/init.sh`:
```bash
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
        # Extract PHP version from Dockerfile if possible
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
```

- [ ] **Step 2: Make init.sh executable**

```bash
chmod +x frank/scripts/init.sh
```

- [ ] **Step 3: Commit init wizard**

```bash
git add frank/scripts/init.sh
git commit -m "feat: add interactive init wizard for frank.yaml creation"
```

---

## Task 8: Create add.sh, remove.sh, and install.sh

**Files:**
- Create: `frank/scripts/add.sh`
- Create: `frank/scripts/remove.sh`
- Create: `frank/scripts/install.sh`

- [ ] **Step 1: Write add.sh**

`frank/scripts/add.sh`:
```bash
#!/usr/bin/env bash
# frank/scripts/add.sh — add a service to frank.yaml and regenerate
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/config.sh"

SERVICE="$1"
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
```

- [ ] **Step 2: Write remove.sh**

`frank/scripts/remove.sh`:
```bash
#!/usr/bin/env bash
# frank/scripts/remove.sh — remove a service from frank.yaml and regenerate
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/config.sh"

SERVICE="$1"
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
```

- [ ] **Step 3: Write install.sh**

`frank/scripts/install.sh`:
```bash
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
    VERSION_ARG="\"$LARAVEL_VERSION\""
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
```

- [ ] **Step 4: Make scripts executable**

```bash
chmod +x frank/scripts/add.sh frank/scripts/remove.sh frank/scripts/install.sh
```

- [ ] **Step 5: Commit**

```bash
git add frank/scripts/add.sh frank/scripts/remove.sh frank/scripts/install.sh
git commit -m "feat: add add/remove/install scripts for service and Laravel management"
```

---

## Task 9: Create the generated justfile template and update the main justfile

**Files:**
- Create: `frank/justfile.tmpl`
- Modify: `justfile` (rewrite as the tool's own justfile)

- [ ] **Step 1: Write justfile.tmpl**

`frank/justfile.tmpl` — this is the template for the justfile generated into user projects:
```just
# Generated by Frank — edit frank.yaml instead
set dotenv-load := true

FRANK_HOME := env_var_or_default("FRANK_HOME", "{{frank_home}}")
docker_compose := "docker compose"

# Default recipe (shows help)
_default:
	@just --list --unsorted

# Initialize Frank project (interactive wizard)
init *ARGS:
	{{FRANK_HOME}}/frank/scripts/init.sh {{ARGS}}

# Regenerate Docker files from frank.yaml
generate *ARGS:
	{{FRANK_HOME}}/frank/scripts/generate.sh {{ARGS}}

# Install Laravel
install:
	@if [ -f artisan ]; then echo "❌ Laravel already installed. Run 'just reset' first."; exit 1; fi
	{{FRANK_HOME}}/frank/scripts/install.sh

# Add a service
add SERVICE:
	{{FRANK_HOME}}/frank/scripts/add.sh {{SERVICE}}

# Remove a service
remove SERVICE:
	{{FRANK_HOME}}/frank/scripts/remove.sh {{SERVICE}}

# Start development environment
up: _maybe_regenerate
	@if [ ! -f artisan ]; then echo "❌ Please run 'just install' first"; exit 1; fi
	@echo "⏳ Starting Frank development environment..."
	@{{docker_compose}} up -d --build
	@{{docker_compose}} exec -T app php artisan migrate --force 2>/dev/null
	@echo "🚀 Laravel is running at http://localhost:8000"

alias start := up

# Stop containers
down:
	@{{docker_compose}} down

alias stop := down

# Stop containers and remove volumes
clean:
	@{{docker_compose}} down -v
	@{{docker_compose}} rm -f

# Reset project (keeps frank.yaml)
reset FORCE="n": clean
	@if [ "{{FORCE}}" = "-f" ]; then \
		confirm="y"; \
	else \
		read -p "⚠️  This will delete all files except frank.yaml and .git. Continue? (y/N): " confirm; \
	fi; \
	if [ "$$confirm" != "y" ]; then \
		echo "❌ Reset aborted."; \
		exit 1; \
	fi; \
	find . -mindepth 1 -maxdepth 1 \
		! -name '.git' \
		! -name 'frank.yaml' \
		! -name '.frank' \
		-exec rm -rf {} +
	@echo "🧹 Project reset."
	@just generate

# Export to Sail-compatible compose
export-sail *ARGS:
	{{FRANK_HOME}}/frank/scripts/sail-export.sh {{ARGS}}

# Generate activation script for shell aliases
_activate:
	@cat .frank/scripts/activate

# Auto-regenerate if frank.yaml is newer than compose.yaml
_maybe_regenerate:
	@if [ -f frank.yaml ] && [ -f compose.yaml ]; then \
		if [ frank.yaml -nt compose.yaml ]; then \
			echo "frank.yaml changed — regenerating..."; \
			just generate; \
		fi; \
	fi

# Generate shell functions — add to shell config with: just shell-setup >> ~/.zshrc
shell-setup:
	@cat .frank/scripts/shell-setup
```

- [ ] **Step 2: Update the main justfile**

The main `justfile` in the Frank tool repo now serves dual purpose: it's used for development of Frank itself AND it can generate the target justfile.

For now, keep it as-is but add a `generate-justfile` recipe:

Add to the existing `justfile`:
```just
# Generate a justfile for a target project
generate-justfile TARGET_DIR:
    @FRANK_HOME="{{justfile_directory()}}" sed "s|{{frank_home}}|{{justfile_directory()}}|g" frank/justfile.tmpl > {{TARGET_DIR}}/justfile
    @echo "✅ justfile generated at {{TARGET_DIR}}/justfile"
```

- [ ] **Step 3: Commit**

```bash
git add frank/justfile.tmpl justfile
git commit -m "feat: add generated justfile template with auto-regeneration"
```

---

## Task 10: Create Sail import/export scripts

**Files:**
- Create: `frank/scripts/sail-import.sh`
- Create: `frank/scripts/sail-export.sh`

- [ ] **Step 1: Write sail-import.sh**

`frank/scripts/sail-import.sh`:
```bash
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
    # Skip the main app service
    [ "$svc" = "laravel.test" ] && continue
    [ "$svc" = "app" ] && continue
    # Check if it matches a known service
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
```

- [ ] **Step 2: Write sail-export.sh**

`frank/scripts/sail-export.sh`:
```bash
#!/usr/bin/env bash
# frank/scripts/sail-export.sh — export frank.yaml as Sail-compatible docker-compose.yml
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/lib/config.sh"

OUTPUT_FILE=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        -f) OUTPUT_FILE="$2"; shift 2 ;;
        *)  echo "Unknown argument: $1" >&2; exit 1 ;;
    esac
done

OUTPUT_FILE="${OUTPUT_FILE:-./docker-compose.yml}"

if ! frank_yaml_exists; then
    echo "Error: frank.yaml not found." >&2
    exit 1
fi

PHP_VERSION=$(frank_config_get '.php.version' "$DEFAULT_PHP_VERSION")
SERVICES=$(frank_services)

echo "📦 Exporting Sail-compatible compose to: $OUTPUT_FILE"

# Start building the Sail compose
cat > "$OUTPUT_FILE" << 'EOF'
# Generated by Frank (Sail export) — this is a best-effort Sail-compatible compose
version: '3'
services:
    laravel.test:
        build:
            context: ./vendor/laravel/sail/runtimes/PHP_VERSION_PLACEHOLDER
            dockerfile: Dockerfile
            args:
                WWWGROUP: '${WWWGROUP}'
        image: sail-PHP_VERSION_PLACEHOLDER/app
        extra_hosts:
            - 'host.docker.internal:host-gateway'
        ports:
            - '${APP_PORT:-80}:80'
            - '${VITE_PORT:-5173}:${VITE_PORT:-5173}'
        environment:
            WWWUSER: '${WWWUSER}'
            LARAVEL_SAIL: 1
            XDEBUG_MODE: '${SAIL_XDEBUG_MODE:-off}'
            XDEBUG_CONFIG: '${SAIL_XDEBUG_CONFIG:-client_host=host.docker.internal}'
            IGNITION_LOCAL_SITES_PATH: '${PWD}'
        volumes:
            - '.:/var/www/html'
        networks:
            - sail
EOF

# Replace PHP version placeholder
sed -i "s/PHP_VERSION_PLACEHOLDER/$PHP_VERSION/g" "$OUTPUT_FILE"

# Add each service in Sail format
for service in $SERVICES; do
    case "$service" in
        pgsql)
            cat >> "$OUTPUT_FILE" << 'EOF'
    pgsql:
        image: 'postgres:17'
        ports:
            - '${FORWARD_DB_PORT:-5432}:5432'
        environment:
            PGPASSWORD: '${DB_PASSWORD:-password}'
            POSTGRES_DB: '${DB_DATABASE}'
            POSTGRES_USER: '${DB_USERNAME}'
            POSTGRES_PASSWORD: '${DB_PASSWORD:-password}'
        volumes:
            - 'sail-pgsql:/var/lib/postgresql/data'
        networks:
            - sail
        healthcheck:
            test: ["CMD", "pg_isready", "-q", "-d", "${DB_DATABASE}", "-U", "${DB_USERNAME}"]
            retries: 3
            timeout: 5s
EOF
            ;;
        mysql)
            cat >> "$OUTPUT_FILE" << 'EOF'
    mysql:
        image: 'mysql/mysql-server:8.0'
        ports:
            - '${FORWARD_DB_PORT:-3306}:3306'
        environment:
            MYSQL_ROOT_PASSWORD: '${DB_PASSWORD}'
            MYSQL_ROOT_HOST: '%'
            MYSQL_DATABASE: '${DB_DATABASE}'
            MYSQL_USER: '${DB_USERNAME}'
            MYSQL_PASSWORD: '${DB_PASSWORD}'
            MYSQL_ALLOW_EMPTY_PASSWORD: 1
        volumes:
            - 'sail-mysql:/var/lib/mysql'
        networks:
            - sail
        healthcheck:
            test: ["CMD", "mysqladmin", "ping", "-p${DB_PASSWORD}"]
            retries: 3
            timeout: 5s
EOF
            ;;
        redis)
            cat >> "$OUTPUT_FILE" << 'EOF'
    redis:
        image: 'redis:alpine'
        ports:
            - '${FORWARD_REDIS_PORT:-6379}:6379'
        volumes:
            - 'sail-redis:/data'
        networks:
            - sail
        healthcheck:
            test: ["CMD", "redis-cli", "ping"]
            retries: 3
            timeout: 5s
EOF
            ;;
        meilisearch)
            cat >> "$OUTPUT_FILE" << 'EOF'
    meilisearch:
        image: 'getmeili/meilisearch:latest'
        ports:
            - '${FORWARD_MEILISEARCH_PORT:-7700}:7700'
        environment:
            MEILI_NO_ANALYTICS: '${MEILISEARCH_NO_ANALYTICS:-false}'
        volumes:
            - 'sail-meilisearch:/meili_data'
        networks:
            - sail
        healthcheck:
            test: ["CMD", "wget", "--no-verbose", "--spider", "http://127.0.0.1:7700/health"]
            retries: 3
            timeout: 5s
EOF
            ;;
        mariadb)
            cat >> "$OUTPUT_FILE" << 'EOF'
    mariadb:
        image: 'mariadb:11'
        ports:
            - '${FORWARD_DB_PORT:-3306}:3306'
        environment:
            MARIADB_ROOT_PASSWORD: '${DB_PASSWORD}'
            MARIADB_ROOT_HOST: '%'
            MARIADB_DATABASE: '${DB_DATABASE}'
            MARIADB_USER: '${DB_USERNAME}'
            MARIADB_PASSWORD: '${DB_PASSWORD}'
            MARIADB_ALLOW_EMPTY_ROOT_PASSWORD: 'yes'
        volumes:
            - 'sail-mariadb:/var/lib/mysql'
        networks:
            - sail
        healthcheck:
            test: ["CMD", "healthcheck.sh", "--connect", "--innodb_initialized"]
            retries: 3
            timeout: 5s
EOF
            ;;
        memcached)
            cat >> "$OUTPUT_FILE" << 'EOF'
    memcached:
        image: 'memcached:alpine'
        ports:
            - '${FORWARD_MEMCACHED_PORT:-11211}:11211'
        networks:
            - sail
EOF
            ;;
        mailpit)
            cat >> "$OUTPUT_FILE" << 'EOF'
    mailpit:
        image: 'axllent/mailpit:latest'
        ports:
            - '${FORWARD_MAILPIT_PORT:-1025}:1025'
            - '${FORWARD_MAILPIT_DASHBOARD_PORT:-8025}:8025'
        networks:
            - sail
EOF
            ;;
    esac
done

# Add networks and volumes
cat >> "$OUTPUT_FILE" << 'EOF'
networks:
    sail:
        driver: bridge
volumes:
EOF

for service in $SERVICES; do
    case "$service" in
        pgsql)       echo "    sail-pgsql:" >> "$OUTPUT_FILE" ;;
        mysql)       echo "    sail-mysql:" >> "$OUTPUT_FILE" ;;
        mariadb)     echo "    sail-mariadb:" >> "$OUTPUT_FILE" ;;
        redis)       echo "    sail-redis:" >> "$OUTPUT_FILE" ;;
        meilisearch) echo "    sail-meilisearch:" >> "$OUTPUT_FILE" ;;
        memcached)   echo "    sail-memcached:" >> "$OUTPUT_FILE" ;;
    esac
done

# Add depends_on to laravel.test
DEPENDS=""
for service in $SERVICES; do
    case "$service" in
        pgsql|mysql|mariadb|redis|meilisearch|memcached)
            DEPENDS="${DEPENDS}\n            - ${service}"
            ;;
    esac
done
if [ -n "$DEPENDS" ]; then
    sed -i "/IGNITION_LOCAL_SITES_PATH/a\\        depends_on:$(echo -e "$DEPENDS")" "$OUTPUT_FILE"
fi

echo "✅ Sail-compatible compose written to: $OUTPUT_FILE"
echo ""
echo "⚠️  This is a best-effort export. You may need to:"
echo "   1. Run 'php artisan sail:install' for full Sail vendor setup"
echo "   2. Review and adjust the generated compose file"
```

- [ ] **Step 3: Make scripts executable**

```bash
chmod +x frank/scripts/sail-import.sh frank/scripts/sail-export.sh
```

- [ ] **Step 4: Commit**

```bash
git add frank/scripts/sail-import.sh frank/scripts/sail-export.sh
git commit -m "feat: add Sail import/export scripts for config interoperability"
```

---

## Task 11: Integration testing — end-to-end workflow

**Files:** No new files — this tests the full pipeline.

- [ ] **Step 1: Test default init + generate**

```bash
mkdir -p /tmp/frank-test && cd /tmp/frank-test
FRANK_HOME=/home/json/code/php/frank /home/json/code/php/frank/frank/scripts/init.sh
# Accept all defaults (press Enter through wizard)
# Verify: frank.yaml exists with defaults
cat frank.yaml
# Verify: compose.yaml, Dockerfile, Caddyfile generated
ls -la compose.yaml Dockerfile Caddyfile .frank/scripts/activate
```

- [ ] **Step 2: Test FPM runtime**

```bash
rm -rf /tmp/frank-test-fpm && mkdir -p /tmp/frank-test-fpm && cd /tmp/frank-test-fpm
cat > frank.yaml << 'EOF'
version: 1
php:
  version: "8.4"
  runtime: fpm
services:
  - mysql
  - redis
  - mailpit
EOF
FRANK_HOME=/home/json/code/php/frank /home/json/code/php/frank/frank/scripts/generate.sh
# Verify: Dockerfile, nginx.Dockerfile, nginx.conf generated (no Caddyfile)
ls -la Dockerfile nginx.Dockerfile nginx.conf
# Verify: compose.yaml has app + nginx + db + redis + mailpit services
yq eval '.services | keys' compose.yaml
```

- [ ] **Step 3: Test validation — conflicting databases**

```bash
cd /tmp/frank-test-fpm
cat > frank.yaml << 'EOF'
version: 1
services:
  - pgsql
  - mysql
EOF
FRANK_HOME=/home/json/code/php/frank /home/json/code/php/frank/frank/scripts/generate.sh
# Expected: Error about multiple database containers
```

- [ ] **Step 4: Test add/remove**

```bash
cd /tmp/frank-test
/home/json/code/php/frank/frank/scripts/add.sh redis
yq eval '.services' frank.yaml
# Expected: pgsql, mailpit, redis

/home/json/code/php/frank/frank/scripts/remove.sh redis
yq eval '.services' frank.yaml
# Expected: pgsql, mailpit
```

- [ ] **Step 5: Test auto-regeneration on up**

```bash
cd /tmp/frank-test
touch -t 202601010000 compose.yaml  # backdate compose.yaml
sleep 1
touch frank.yaml  # make frank.yaml newer
# The _maybe_regenerate recipe in justfile would detect staleness
# Verify the logic:
[ frank.yaml -nt compose.yaml ] && echo "STALE — would regenerate" || echo "UP TO DATE"
```

- [ ] **Step 6: Clean up test directories**

```bash
rm -rf /tmp/frank-test /tmp/frank-test-fpm
```

- [ ] **Step 7: Commit any fixes from integration testing**

```bash
git add -A
git commit -m "fix: integration test fixes for generate pipeline"
```

---

## Task 12: Update README and clean up old files

**Files:**
- Modify: `README.md`
- Remove: `docker-compose.yml`, `Dockerfile`, `Caddyfile` (now generated)
- Remove: `frank/scripts/laravel-init.sh` (replaced by install.sh)

- [ ] **Step 1: Remove old hardcoded files**

These files are now generated by `just generate`. Remove them from the repo:

```bash
git rm docker-compose.yml Dockerfile Caddyfile
git rm frank/scripts/laravel-init.sh
```

- [ ] **Step 2: Update README.md**

Rewrite the README to reflect the new config-driven workflow. Key sections:
- Updated requirements (Docker + Just + yq)
- New quick start: `just init` → `just install` → `just up`
- `frank.yaml` documentation
- Service selection
- Sail interop
- Migration from old Frank

- [ ] **Step 3: Commit cleanup**

```bash
git add -A
git commit -m "feat: remove hardcoded Docker files, update README for config-driven Frank"
```
