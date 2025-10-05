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
	@echo "💡 Run 'source <(just activate)' to set up shell aliases for composer and artisan"

# Generate activation script for shell aliases (like Python venv)
[doc('Generate shell activation script - use with: source <(just activate)')]
activate:
	@echo "# Laravel Docker Environment Activation"
	@echo "# This script sets up aliases for Laravel development"
	@echo ""
	@echo "# Check if containers are running"
	@echo "if ! {{docker_compose}} ps app | grep -q 'Up'; then"
	@echo "  echo '❌ Laravel containers are not running. Please run \"just up\" first.'"
	@echo "  return 1"
	@echo "fi"
	@echo ""
	@echo "# Set up aliases"
	@echo "alias composer='{{docker_compose}} exec app composer'"
	@echo "alias artisan='{{docker_compose}} exec app php artisan'"
	@echo ""
	@echo "# Set up environment variable to indicate activation"
	@echo "export LARAVEL_DOCKER_ENV_ACTIVE=1"
	@echo ""
	@echo "# Update prompt to show activation (optional)"
	@echo "if [[ -z \"$$LARAVEL_DOCKER_ENV_ORIGINAL_PS1\" ]]; then"
	@echo "  export LARAVEL_DOCKER_ENV_ORIGINAL_PS1=\"$$PS1\""
	@echo "fi"
	@echo "export PS1=\"(laravel-docker) $$LARAVEL_DOCKER_ENV_ORIGINAL_PS1\""
	@echo ""
	@echo "# Function to deactivate"
	@echo "deactivate() {"
	@echo "  unalias composer 2>/dev/null || true"
	@echo "  unalias artisan 2>/dev/null || true"
	@echo "  if [[ -n \"$$LARAVEL_DOCKER_ENV_ORIGINAL_PS1\" ]]; then"
	@echo "    export PS1=\"$$LARAVEL_DOCKER_ENV_ORIGINAL_PS1\""
	@echo "    unset LARAVEL_DOCKER_ENV_ORIGINAL_PS1"
	@echo "  fi"
	@echo "  unset LARAVEL_DOCKER_ENV_ACTIVE"
	@echo "  unset -f deactivate"
	@echo "  echo '📦 Laravel Docker environment deactivated'"
	@echo "}"
	@echo ""
	@echo "echo '🚀 Laravel Docker environment activated!'"
	@echo "echo '📦 Available commands: composer, artisan'"
	@echo "echo '🔧 To deactivate, run: deactivate'"

alias stop := down

# Stop containers
down:
	@{{docker_compose}} down
	@if [ -n "$$LARAVEL_DOCKER_ENV_ACTIVE" ]; then \
		echo "💡 {{BG_YELLOW}}Don't forget to run '{{BOLD + BLUE}}deactivate{{NORMAL + BG_YELLOW + WHITE}}' to clean up your shell aliases{{NORMAL}}"; \
	fi

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
		! -name 'scripts' \
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
