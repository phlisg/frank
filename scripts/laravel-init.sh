#!/bin/sh
if [  -f 'artisan' ]; then
    echo 'Laravel project already exists, exiting...';
    exit 0;
fi

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

# Use the current folder name as the application name
folder_name=$(basename "$PWD")

# Modify .env file for database configuration using postgres
sed -i 's/DB_CONNECTION=mysql/DB_CONNECTION=pgsql/' .env
sed -i 's/# DB_HOST=127.0.0.1/DB_HOST=db/' .env
sed -i 's/# DB_PORT=3306/DB_PORT=5432/' .env
sed -i "s/# DB_DATABASE=laravel/DB_DATABASE=$folder_name/" .env
sed -i "s/# DB_USERNAME=root/DB_USERNAME=root/" .env
sed -i "s/# DB_PASSWORD=/DB_PASSWORD=root/" .env

# Add DB_URL for PostgreSQL connection
sed -i "/^DB_CONNECTION=/a DB_URL=postgresql://$folder_name:$folder_name@db:5432/$folder_name" .env

# Change URL
sed -i "s|APP_URL=http://localhost|APP_URL=http://localhost:8000|" .env

echo 'Laravel project created!';

# Restore existing README.md 
mv README.frank.md README.md
cat .gitignore.frank >> .gitignore
rm .gitignore.frank