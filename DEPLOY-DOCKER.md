# Deploying with Docker

This guide runs Roster Tracker as a single Docker container. The image contains one Go binary with the web UI embedded, and stores its SQLite database and uploaded files in one data directory that you mount from the host.

## What you need

- A server with Docker Engine 20.10+ and the Docker Compose plugin (`docker compose version`).
- To **build** the image (on the server or on your own machine): Git, [Node.js](https://nodejs.org/) 24 and [pnpm](https://pnpm.io/) 11. The image does not build the web UI itself; you build it first and the Docker build embeds it.
- About 1 GB of free disk to build, plus room for the data directory (database and uploads).

## 1. Get the code

```bash
git clone https://github.com/anangnovriadi/roster-tracker.git
cd roster-tracker
```

## 2. Build the web UI

The Go binary embeds the files in `server/frontend/dist`, so build them before every image build:

```bash
cd web
pnpm install
pnpm release
cd ..
```

`pnpm release` writes the production bundle to `server/frontend/dist`. Run it again whenever the UI changes.

## 3. Configure

Create a `.env` file next to `scripts/compose.yaml` (`scripts/.env`). Every value is optional:

```bash
# Version label shown in the app
ROSTER_VERSION=26.10

# Time zone for logs, e.g. Asia/Jakarta
TZ=Asia/Jakarta

# Public URL of the app, without a trailing path. Leave empty for a local test.
ROSTER_INSTANCE_URL=https://roster.example.com

# Where the database and uploads live on the host
ROSTER_DATA_DIR=/opt/roster-tracker/data

# Host port to expose (the container always listens on 5230)
ROSTER_HOST_PORT=5230
```

Create the data directory once:

```bash
sudo mkdir -p /opt/roster-tracker/data
```

The container starts as root only long enough to give that directory to its unprivileged user (UID/GID 10001), then drops privileges.

## 4. Build and start

```bash
docker compose -f scripts/compose.yaml up -d --build
```

Check that it is running and healthy:

```bash
docker compose -f scripts/compose.yaml ps
docker compose -f scripts/compose.yaml logs -f roster-tracker
curl http://localhost:5230/healthz
```

Open `http://203.0.113.10:5230` (replace with your server's IP).

## 5. First sign-in

On the first start, with an empty database, the server creates one admin account:

| Field | Value |
| --- | --- |
| Username | `admin` |
| Email | `admin@mail.com` |
| Password | `12345678` |

1. Sign in with `admin` (or `admin@mail.com`) and `12345678`.
2. **Change the password immediately** in Settings → My Account.
3. Add members in Settings → Member (or with the **+** in the sidebar).

The admin manages members and the duty roster and can read every note; only members write notes, comments and reactions. Visitors who are not signed in can see the roster and PUBLIC notes, read-only.

## Running without Compose

If you prefer plain `docker` commands:

```bash
# Build (after `pnpm release`)
docker build -f scripts/Dockerfile \
  --build-arg VERSION=26.10 \
  --build-arg COMMIT="$(git rev-parse --short HEAD)" \
  -t roster-tracker:local .

# Run
docker run -d --name roster-tracker \
  --restart unless-stopped \
  -p 5230:5230 \
  -e TZ=Asia/Jakarta \
  -e ROSTER_INSTANCE_URL=https://roster.example.com \
  -v /opt/roster-tracker/data:/var/opt/roster-tracker \
  roster-tracker:local
```

## Building on another machine

If the server should not have Node.js installed, build the image on your own machine and copy it over:

```bash
# On your machine (after `pnpm release` and `docker build ... -t roster-tracker:local .`)
docker save roster-tracker:local | gzip > roster-tracker.tar.gz
scp roster-tracker.tar.gz user@server:/tmp/

# On the server
gunzip -c /tmp/roster-tracker.tar.gz | docker load
docker run ...   # same command as above
```

When the build machine and the server have different CPU architectures (for example an Apple Silicon Mac and an x86 server), build for the server's platform:

```bash
docker buildx build --platform linux/amd64 -f scripts/Dockerfile \
  --build-arg VERSION=26.10 -t roster-tracker:local --load .
```

## Configuration reference

Every server flag can be set as an environment variable with the `ROSTER_` prefix (dashes become underscores):

| Variable | Description | Default in the image |
| --- | --- | --- |
| `ROSTER_PORT` | Port inside the container | `5230` |
| `ROSTER_INSTANCE_URL` | Public URL of the app (http or https, no query string) | empty |
| `ROSTER_DRIVER` | `sqlite`, `mysql` or `postgres` | `sqlite` |
| `ROSTER_DSN` | Database connection string (required for MySQL/PostgreSQL) | `roster_prod.db` in the data directory |
| `ROSTER_DSN_FILE` | Read the DSN from a file instead (Docker secrets) | — |
| `ROSTER_LOG_LEVEL` | `debug`, `info`, `warn` or `error` | `info` |
| `ROSTER_TRUSTED_PROXIES` | Proxies whose forwarding headers are trusted | `private` |
| `ROSTER_UID` / `ROSTER_GID` | User the server runs as inside the container | `10001` |
| `TZ` | Container time zone | `UTC` |

### Using MySQL or PostgreSQL

SQLite is the default and is enough for a small team. To use a database server instead:

```bash
ROSTER_DRIVER=postgres
ROSTER_DSN=postgresql://user:password@db-host:5432/roster_tracker?sslmode=disable
```

Uploaded files still go to the data directory (or to S3 if you configure it), so keep the volume mounted.

## Access by IP address (no domain)

A domain is not required. The examples below use `203.0.113.10`, an address reserved for documentation; replace it with your server's public IP. To reach the app at `http://203.0.113.10:5230`:

1. In `scripts/.env`, set the instance URL to that address (or leave it empty):

   ```bash
   ROSTER_INSTANCE_URL=http://203.0.113.10:5230
   ROSTER_HOST_PORT=5230
   ```

2. Open the port in the server firewall, e.g. with `ufw`:

   ```bash
   sudo ufw allow 5230/tcp
   ```

   On a cloud server, also allow inbound TCP 5230 in the provider's security group or firewall.

3. Start the app as in step 4 and open `http://203.0.113.10:5230`.

Signing in works over plain HTTP: the session cookie is only marked `Secure` when the request arrives over HTTPS.

Over plain HTTP, passwords and notes travel unencrypted. That is fine on a private network or VPN, but on the public internet prefer a domain with HTTPS (next section). If the server's IP changes, update `ROSTER_INSTANCE_URL`; it is only used for links in emails and the sitemap.

## HTTPS with a reverse proxy

Put the container behind a reverse proxy that terminates TLS and set `ROSTER_INSTANCE_URL` to the public `https://` address. Bind the container port to localhost only so it is reachable only through the proxy, e.g. `ROSTER_HOST_PORT=127.0.0.1:5230`.

[Caddy](https://caddyserver.com/) gets certificates automatically:

```
roster.example.com {
    reverse_proxy 127.0.0.1:5230
}
```

Nginx:

```nginx
server {
    listen 443 ssl;
    server_name roster.example.com;
    # ssl_certificate / ssl_certificate_key ...

    client_max_body_size 100m;   # allow large uploads

    location / {
        proxy_pass http://127.0.0.1:5230;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        # Live updates use server-sent events; do not buffer them.
        proxy_buffering off;
        proxy_read_timeout 1h;
    }
}
```

## Data and backups

Everything that matters is in the data directory (`ROSTER_DATA_DIR`):

| Path | Contents |
| --- | --- |
| `roster_prod.db` (+ `-wal`, `-shm`) | SQLite database: users, roster, notes, settings |
| `assets/` | Uploaded attachments |

Back up **both**. For a consistent SQLite copy, stop the container first:

```bash
docker compose -f scripts/compose.yaml stop
tar czf roster-backup-$(date +%F).tar.gz -C /opt/roster-tracker data
docker compose -f scripts/compose.yaml start
```

Restore by stopping the container, replacing the data directory with the backup, and starting it again.

Uploads have no size limit by default. To cap them, add a usage-limit deployment file to the data directory (`roster-tracker-instance-setting-usage-limits.json`).

## Updating

```bash
cd roster-tracker
git pull
cd web && pnpm install && pnpm release && cd ..
docker compose -f scripts/compose.yaml up -d --build
```

Database migrations run automatically on start. Take a backup before updating.

## Starting over

To wipe all data and get a fresh admin account:

```bash
docker compose -f scripts/compose.yaml down
sudo rm -rf /opt/roster-tracker/data/*
docker compose -f scripts/compose.yaml up -d
```

This deletes every member, note, roster day and upload.

## Troubleshooting

| Problem | Fix |
| --- | --- |
| Build fails with `pattern dist/*: no matching files found` | The web UI was not built. Run `cd web && pnpm release` before building the image. |
| Build fails with `Pass VERSION ...` | Add `--build-arg VERSION=26.10` (Compose sets it from `ROSTER_VERSION`). |
| `permission denied` on the data directory | Make sure the host directory exists; the entrypoint gives it to UID 10001. Under rootless Docker set `ROSTER_UID=0` and `ROSTER_GID=0`. |
| Page loads but live updates stall behind a proxy | Disable proxy buffering for the app (see the Nginx example). |
| Visitors who are not signed in see a sign-in page | The instance access mode was changed to private; the seeded instance is public. |
