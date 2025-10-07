## 🐘 Frank
> Or the LFDJ Stack (Laravel + FrankenPHP + Docker + Just)

A minimal setup for running a Laravel 12 application with [FrankenPHP](https://frankenphp.dev/), Docker, and [just](https://just.systems) for task automation, without you needing to install PHP, FrankenPHP, Composer, etc. Admittedly you still need Docker and Just installed, but I find those an acceptable minimum.

Comes with Mailjet & PostgreSQL 😎

---

### 📋 Todo:

- [x] Install node dependencies the same way or similar
- [x] Set up database in `docker-compose.yml`
- [x] Added convenience aliases for QoL (`up`, `down`, `composer`, `php`, `npm`, `artisan` and `psql`)
- [ ] Add various other tools/Laravel plugins?
    - [ ] Sail support (easier to manage php versions)
    - [ ] Octane out of the box support? (might make dev a bit harder)

---

### 📦 Requirements

* Docker
* [just](https://just.systems) (task runner)

> This repo was solely tested on a Fedora 42+ system. I would recommend running this repo either in WSL or macOS.

---

### 🚀 Quick Start

#### 1. Setup

```bash
just install
```

You should run this command right after creating this repository. This will create a full laravel initial installation. 

> It is important you run this recipe first as to avoid creating a database with wrong credentials (among other things).

#### 2. Use shell aliases (convenience aliases)

> [!WARNING]
> Ignore this step if you already have the functions `up` and `down` in your terminal profile.

To make development easier and add contextual aliases to your shell, you may run:

```bash
just shell-setup >> ~/.zshrc # or ~/.bashrc
source ~/.zshrc # or ~/.bashrc
```

To add two functions `up` and `down`. 

- `up` : starts the containers and sources aliases for composer, artisan and psql to the dockerized application
- `down`: stops the containers and removes the aliases

> [!TIP]
> These convenient functions will save you a few keystrokes when interacting with your containers. `composer` here is `docker compose exec app composer` and `artisan` is `docker compose exec app php artisan`.

#### 3. Start the Development Environment

Once the install has completed, you may start the development environment with:

```bash
up
# or if you don't have the convenience aliases installed:
just up
```

This will create the other containers, and run migrations.

You can now visit: [http://localhost:8000](http://localhost:8000)

### 🛠 Common Commands

| Command                   | Description                                          |
| ------------------------- | ---------------------------------------------------- |
| `just install`            | Install Laravel                                      |
| `just up`                 | Start the development environment                    |
| `just down`               | Stop containers                                      |
| `just clean`              | Stop containers and remove volumes                   |
| `just reset`              | **Deletes all files** except core config files       |

---

### ⚠️ Project reset 

You can reset the whole repository with `just reset`. This command is mostly useful is something went bad during install or during template development.

This command **deletes everything in the project directory** and restores the project back to how it was mostly looking when initially pulled.

You’ll be prompted to confirm before anything is deleted.

