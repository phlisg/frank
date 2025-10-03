## 🐘 Frank
> Or the LFDJ Stack (Laravel + FrankenPHP + Docker + Just)

A minimal setup for running a Laravel 12 application with [FrankenPHP](https://frankenphp.dev/), Docker, and [just](https://just.systems) for task automation, without you needing to install PHP, FrankenPHP, Composer, etc. Admittedly you still need Docker and Just installed, but I find those an acceptable minimum.

Comes with Mailjet & PostgreSQL 😎

---

### 📋 Todo:

- [ ] Install node dependencies the same way or similar?
- [x] Set up database in `docker-compose.yml`
- [ ] Add various other tools/Laravel plugins?

---

### 📦 Requirements

* Docker
* [just](https://just.systems) (task runner)

> This repo was mostly tested on a Fedora 42+ system.

---

### 🚀 Quick Start

#### 1. Setup

```bash
just install
```

You should run this command right after creating this repository. This will create a full laravel initial installation. 

> It is important you run this recipe first as to avoid creating a database with wrong credentials (among other things).


#### 2. Start Development Environment

Once the install has completed, you may start the development environment with:

```bash
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

