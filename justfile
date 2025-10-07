## General comments
#
# If you wish to run recipes after another, precede the recipe name with '&&', for example: "recipe: && other-recipe"
#
## 

set dotenv-load := true

# Set docker-compose as a variable
docker_compose := "docker compose"

# Default recipe (shows help)
[doc]
_default:
	@just --list --unsorted


# You should run this command just after creating the repository to avoid building containers. It will install Laravel. 
[doc('Install laravel')]
install:
	@if [ -f artisan ]; then echo "❌ Laravel already installed. If you want to reinstall, please run 'just reset' first."; exit 1; fi
	@FOLDER_NAME="$(basename "{{justfile_directory()}}")" {{docker_compose}} up --build laravel-init
	@echo "✅ Laravel installation complete."
	@echo "🎉 You can now start the development environment with 'just up'."

[doc]
_build:
	@{{docker_compose}} build 

alias start := up 
# Start development environment
up: _build
	@if [ ! -f artisan ]; then echo "❌ Please run 'just install' first"; exit 1; fi
	@echo "⏳ Starting Laravel development environment..."
	@{{docker_compose}} up -d --build
	@{{docker_compose}} exec -T app php artisan migrate --force 2>/dev/null
	@echo "🚀 Laravel is running at http://localhost:8000"

# Generate activation script for shell aliases (like Python venv)
[doc('Generate shell activation script - use with: source <(just _activate)')]
_activate:
	./frank/scripts/activate

alias stop := down

# Stop containers
down:
	@{{docker_compose}} down

# Clean containers and remove volumes
clean:
	@{{docker_compose}} down -v
	@{{docker_compose}} rm -f


alias rm := reset 

# Reset project files (except key config files)
reset FORCE: clean
	@if [ "{{FORCE}}" = "-f" ]; then \
		confirm="y"; \
	else \
		read -p "⚠️  This will delete all files except .dockerignore, Caddyfile, docker-compose.yml, Dockerfile, and justfile. It will also reset any git changes. Continue? (y/N): " confirm; \
	fi; \
	if [ "$confirm" != "y" ]; then \
		echo "❌ Reset aborted."; \
		exit 1; \
	fi; \
	find . -mindepth 1 -maxdepth 1 \
		! -name '.dockerignore' \
		! -name '.git' \
		! -name 'frank' \
		! -name 'Caddyfile' \
		! -name 'docker-compose.yml' \
		! -name 'Dockerfile' \
		! -name 'justfile' \
		! -name 'README.md' \
		! -name '.gitignore' \
		-exec rm -rf {} + 
	@echo "🧹 Project reset — all generated files removed."
	@if ! git diff --quiet .gitignore; then \
		git restore .gitignore; \
		echo "🔄 .gitignore was modified and has been restored."; \
	else \
		echo "✅ .gitignore was not modified."; \
	fi;

# Generate shell function for automatic activation on up/down
[doc('Generate shell functions for automatic aliases (up/down) - add to your shell config with just shell-setup >> ~/.zshrc or ~/.bashrc')]
shell-setup:
	./frank/scripts/shell-setup