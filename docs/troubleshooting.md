# Troubleshooting

## `frank up` stopped my other project

**Symptom:** Starting a project prints `✓ Stopping previously active project <name>` and the containers of a different project go down.

**Cause:** Not a bug. Frank publishes fixed host ports (443, 5432, 8025, …) for normal projects, so only one can run at a time. Frank records the running project in `~/.local/state/frank/active-project.json` and stops it before starting the next one — otherwise the second `frank up` would simply fail to bind those ports.

**Fix:** If you need both running at once, make one a [worktree](worktrees.md). Worktrees use ephemeral ports, co-exist by design, and are exempt from the auto-stop. There is no flag to disable it.

## Frank says it's stopping a project that isn't running

**Symptom:** `frank up` announces it's stopping a project, warns that it couldn't, and then starts normally.

**Cause:** The state file points at a project that was stopped some other way — `frank compose down`, `docker compose down`, a Docker restart, or a deleted project directory. Frank doesn't hook every path that can stop containers.

**Fix:** Nothing. The warning is cosmetic; Frank clears the stale pointer and continues. If you want to reset it by hand, delete `~/.local/state/frank/active-project.json`.

## Vite dev server CORS errors (fpm runtime)

**Symptom:** Running `npm run dev` works without errors, but the browser reports CORS failures when loading `http://localhost:5173/@vite/client` or `http://localhost:5173/resources/js/app.js`. Status code shows `(null)`, meaning the connection was refused rather than rejected with a response.

**Cause:** The compose template maps port 5173 to the nginx container, but the generated `nginx.conf` has no server block listening on 5173. Requests from the browser hit nginx and get refused before reaching the Vite dev server running inside `laravel.test`.

**Fix:** The `nginx.conf.tmpl` for the fpm runtime now includes a proxy server block on port 5173:

```nginx
server {
    listen 5173;
    server_name _;

    location / {
        proxy_pass http://laravel.test:5173;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
    }
}
```

If you hit this on an existing project (before the template was fixed), add the block above to `.frank/nginx.conf` manually and rebuild the nginx container:

```bash
frank up -d
```
