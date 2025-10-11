FROM dunglas/frankenphp AS base

# add additional extensions here:
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
    zip \
    unzip \
    && docker-php-ext-configure pgsql -with-pgsql=/usr/local/pgsql \
    && docker-php-ext-install pdo_mysql pdo_pgsql pgsql mbstring exif pcntl bcmath gd intl zip \
    && npm install -g npm pnpm bun \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# Copy the Caddyfile
COPY Caddyfile /etc/caddy/Caddyfile

# Set proper permissions script
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

# Expose port 8000 for FrankenPHP and 5173 for Vite dev server
EXPOSE 8000 5173

# Set entrypoint and command
ENTRYPOINT ["/entrypoint.sh"]
CMD ["frankenphp", "run", "--config", "/etc/caddy/Caddyfile"]