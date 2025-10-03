# Set docker-compose as a variable
docker_compose := "docker compose"

# Default recipe (shows help)
[doc]
default:
	@just --list --unsorted


# You should run this command just after creating the repository to avoid building containers. The special "&& up" expression signifies the recipe will run after install, but setup will run before. 
[doc('Install laravel')]
install: && up
	@if [ -f artisan ]; then echo "❌ Laravel already installed. If you want to reinstall, please run 'just reset' first."; exit 1; fi
	{{docker_compose}} up -d laravel-init
	@echo "✅ Laravel installation complete."
	

# Start development environment
up:
	@if [ ! -f artisan ]; then echo "❌ Please run 'just install' first"; exit 1; fi
	{{docker_compose}} up -d
	@echo "🚀 Laravel is running at http://localhost:8000"

# Stop containers
down:
	{{docker_compose}} down

# Clean environment
clean:
	{{docker_compose}} down -v
	{{docker_compose}} rm -f

# Reset project files (except key config files)
reset: clean
    @read -p "⚠️  This will delete all files except .dockerignore, Caddyfile, docker-compose.yml, Dockerfile, and justfile. It will also reset any git changes. Continue? (y/N): " confirm; \
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
    @echo "⚠️ You might need to manually reset the .gitignore file."
