# PHP Runtimes

[← Back to README](../README.md)

Frank supports two runtimes. The right choice depends on how close you want your dev environment to match production.

**`frankenphp` (default) — recommended for most projects**

FrankenPHP is a modern PHP app server built on top of Caddy. It runs your Laravel app in a single container with HTTP/2 and HTTPS out of the box. It's fast to start, simple to configure, and great for greenfield projects or teams that don't have a strong opinion about their production stack.

Choose `frankenphp` if: you're starting a new project, you deploy to a platform like Laravel Cloud or Fly.io, or you just want things to work with minimal fuss.

**`fpm` — for production parity with traditional stacks**

PHP-FPM pairs with a separate Nginx container, matching the classic shared-hosting and VPS setup. More moving parts, but familiar if your production server runs Nginx + PHP-FPM.

Choose `fpm` if: your production environment uses Nginx + PHP-FPM, or you're maintaining an existing project that was built with that stack in mind.

| Runtime | Containers | Best for |
| ------- | ---------- | -------- |
| `frankenphp` | Single (app) | New projects, modern deployments |
| `fpm` | Two (app + nginx) | Production parity with Nginx stacks |

## FPM notes

Frank runs php-fpm in foreground mode. PHP 8.5+ no longer accepts the legacy `daemonize` directive, so do not add it to custom pool configs — php-fpm will refuse to start with `unknown entry 'daemonize'`.
