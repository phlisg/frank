# Set docker-compose as a variable
docker_compose := "docker compose"

# Default recipe (shows help)
default:
	@just --summary --unsorted

# Show help
help:
	@echo "Laravel FrankenPHP Development Commands:"
	@echo "  just setup       - First-time setup (creates .env.docker with your user ID)"
	@echo "  just up          - Start the development environment"
	@echo "  just down        - Stop the development environment"
	@echo "  just build       - Build/rebuild containers"
	@echo "  just logs        - Show application logs"
	@echo "  just shell       - Access the container shell"
	@echo "  just artisan     - Run artisan command (usage: just artisan cmd='make:controller Test')"
	@echo "  just composer    - Run composer command (usage: just composer cmd='require package/name')"
	@echo "  just clean       - Stop containers and remove volumes"
	@echo "  just reset       - Delete all generated files except project config files"

# First-time setup
setup:
	@echo "UID=$(id -u)" > .env.docker
	@echo "GID=$(id -g)" >> .env.docker
	@echo "✅ Created .env.docker with UID=$(id -u) and GID=$(id -g)"
	@echo "Now you can run: just up"

# Build containers
build:
	{{docker_compose}} build

# Start development environment
up: setup
	{{docker_compose}} up -d
	@echo "🚀 Laravel is running at http://localhost:8000"

# Stop containers
down:
	{{docker_compose}} down

# Show logs
logs:
	{{docker_compose}} logs -f app

# Access container shell
shell:
	{{docker_compose}} exec app bash

# Run artisan command: just artisan cmd='foo'
artisan cmd:
	{{docker_compose}} exec app php artisan {{cmd}}

# Run composer command: just composer cmd='require vendor/package'
composer cmd:
	{{docker_compose}} exec app composer {{cmd}}

# Clean environment
clean:
	{{docker_compose}} down -v
	{{docker_compose}} rm -f

# Reset project files (except key config files)
reset: clean
    @read -p "⚠️  This will delete all files except .dockerignore, Caddyfile, docker-compose.yml, Dockerfile, and justfile. Continue? (y/N): " confirm; \
    if [ "$confirm" != "y" ]; then \
        echo "❌ Reset aborted."; \
        exit 1; \
    fi; \
    find . -mindepth 1 -maxdepth 1 \
        ! -name '.dockerignore' \
        ! -name '.git' \
        ! -name 'Caddyfile' \
        ! -name 'docker-compose.yml' \
        ! -name 'Dockerfile' \
        ! -name 'justfile' \
        ! -name 'README.md' \
        ! -name '.gitignore' \
        -exec rm -rf {} +
    @echo "🧹 Project reset — all generated files removed."
