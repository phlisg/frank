## 🐘 Frank
> Or the LFDJ Stack (Laravel + FrankenPHP + Docker + Just)

A minimal setup for running a Laravel 12 application with [FrankenPHP](https://frankenphp.dev/), Docker, and [just](https://just.systems) for task automation, without you needing to install PHP, FrankenPHP, Composer, etc. Admittedly you still need Docker and Just installed, but I find those an acceptable minimum.

---

### 📋 Todo:

- [ ] Install node dependencies the same way or similar
- [ ] Set up database in `docker-compose.yml`
- [ ] Add various other tools/Laravel plugins

---

### 📦 Requirements

* Docker
* [just](https://just.systems) (task runner)
* GNU/Linux system (e.g. Fedora 42) — tested on systems with strict user permissions

---

### 🚀 Quick Start

#### 1. Setup

```bash
just setup
```

This creates a `.env.docker` file with your user ID and group ID to ensure files created in Docker are owned by you.

#### 2. Build Containers

```bash
just build
```

#### 3. Start Development Environment

```bash
just up
```

Visit: [http://localhost:8000](http://localhost:8000)

---

> [!TIP]
> You can just run `just up` directly instead too :)

### 🛠 Common Commands

| Command                   | Description                                          |
| ------------------------- | ---------------------------------------------------- |
| `just up`                 | Start the development environment (runs setup first) |
| `just down`               | Stop containers                                      |
| `just build`              | Build/rebuild containers                             |
| `just logs`               | Tail application logs                                |
| `just shell`              | Open a shell inside the app container                |
| `just artisan cmd='...'`  | Run Laravel Artisan commands                         |
| `just composer cmd='...'` | Run Composer commands inside the container           |
| `just clean`              | Stop containers and remove volumes                   |
| `just reset`              | **Deletes all files** except core config files       |

---

### ⚠️ `just reset`

This command **deletes everything in the project directory** and restores the project back to how it was mostly looking when initially pulled.

You’ll be prompted to confirm before anything is deleted.

> [!WARNING]
> You will still need to revert changes to `.gitignore` and `README.md`


