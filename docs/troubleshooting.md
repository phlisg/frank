# Troubleshooting

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
