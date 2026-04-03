#!/bin/sh
set -e

LARAVEL_VERSION="${1:-}"

# Skip if already installed.
if [ -f artisan ]; then
    echo "Laravel is already installed, skipping."
    exit 0
fi

# Install Laravel into a temp subfolder (current dir may not be empty).
echo "Running composer create-project..."
if [ -n "$LARAVEL_VERSION" ] && [ "$LARAVEL_VERSION" != "latest" ]; then
    composer create-project --prefer-dist laravel/laravel .temp-laravel "$LARAVEL_VERSION" --no-interaction
else
    composer create-project --prefer-dist laravel/laravel .temp-laravel --no-interaction
fi

# Move everything to the project root.
cp -a .temp-laravel/. .
rm -rf .temp-laravel

echo "Laravel installed successfully."
