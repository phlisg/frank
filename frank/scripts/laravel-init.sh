#!/bin/sh
if [  -f 'artisan' ]; then
    echo 'Laravel project already exists, exiting...';
    exit 0;
fi

# Use the current folder name as the application name, passed by the compose file
folder_name=$1

echo "Setting up Laravel with database name: $folder_name"

# Backup existing files
mv README.md README.frank.md
mv .gitignore .gitignore.frank

echo 'Creating new Laravel project...';
composer create-project --prefer-dist laravel/laravel .temp-laravel;

# Copying files back to host
cp -r .temp-laravel/* .;
cp .temp-laravel/.env.example . 2>/dev/null || true;
cp .env.example .env 2>/dev/null || true;
cp .temp-laravel/.gitignore . 2>/dev/null || true;

# Remove temporary directory
rm -rf .temp-laravel;

# Some laravel stuff
php artisan key:generate;

# Modify .env file for database configuration using postgres
echo "Configuring database settings..."
sed -i 's/DB_CONNECTION=sqlite/DB_CONNECTION=pgsql/' .env
sed -i 's/# DB_HOST=127\.0\.0\.1/DB_HOST=db/' .env
sed -i 's/# DB_PORT=3306/DB_PORT=5432/' .env
echo "Setting database name to: $folder_name"
sed -i "s|# DB_DATABASE=laravel|DB_DATABASE=$folder_name|" .env
sed -i 's/# DB_USERNAME=root/DB_USERNAME=root/' .env
sed -i 's/# DB_PASSWORD=/DB_PASSWORD=root/' .env

# Add DB_URL for PostgreSQL connection
sed -i '/^DB_PASSWORD=/a DB_URL=postgresql://${DB_USERNAME}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_DATABASE}' .env

# Change URL
sed -i "s|APP_URL=http://localhost|APP_URL=http://localhost:8000|" .env

# Update vite.config.js for HMR
echo "Updating vite.config.js for HMR..."
# Insert server config into vite.config.js inside defineConfig({...}), keeping existing config
if ! grep -q "server:" vite.config.js; then
    sed -i "/defineConfig({/a \    server: {\n        host: '0.0.0.0',\n        port: 5173,\n        hmr: {\n            host: 'localhost',\n        },\n    }," vite.config.js
fi

echo '🎈 Laravel project created!';

# Restore existing README.md 
mv README.frank.md README.md
cat .gitignore.frank >> .gitignore
rm .gitignore.frank