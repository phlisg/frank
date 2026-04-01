#!/bin/sh
set -e

LARAVEL_VERSION="${1:-}"

# Skip if already installed.
if [ -f artisan ]; then
    echo "Laravel is already installed, skipping."
    exit 0
fi

# Back up Frank's files so composer doesn't overwrite them.
[ -f README.md ]  && mv README.md  README.frank.md
[ -f .gitignore ] && mv .gitignore .gitignore.frank

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

# Restore Frank's files. Always remove Laravel's README — if Frank had one,
# restore it; otherwise leave the project without a README (user owns that).
rm -f README.md
[ -f README.frank.md ] && mv README.frank.md README.md
if [ -f .gitignore.frank ]; then
    cat .gitignore.frank >> .gitignore
    rm .gitignore.frank
fi

echo "Laravel installed successfully."
