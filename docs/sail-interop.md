# Sail Interop

[← Back to README](../README.md)

Frank can read and write Laravel Sail's `docker-compose.yml` format, making it easy to migrate an existing Sail project to Frank or hand a Frank project off to someone who prefers Sail.

**Migrating from Sail:**

```bash
frank import              # reads ./docker-compose.yml
frank import -f path/to/docker-compose.yml
```

Frank inspects the Sail compose file, detects your PHP version and services, writes `frank.yaml`, and regenerates all Docker files. Your existing Sail compose file is not modified.

**Ejecting to Sail:**

```bash
frank eject
```

Installs Laravel Sail into the running containers (`composer require laravel/sail` + `sail:install`) using the services from your `frank.yaml`. Useful for handing a project off to a team that prefers Sail. Requires containers to be running — run `frank up` first, then run `./vendor/bin/sail up` to continue with Sail.
