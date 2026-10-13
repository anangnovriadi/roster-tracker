# Deploying with Docker

You build the Docker image on your own machine. The server only needs **two files**, `docker-compose.yml` and `.env`. It does not need the source code, Node.js or Go.

```
Your machine                                   Server
────────────                                   ──────
source code ── scripts/build-image.sh ──▶ image ──▶ docker-compose.yml + .env
                                         (registry or file)    └─ data/ (database + uploads)
```

The image contains one Go binary with the web UI embedded. The SQLite database and uploaded files live in a `data/` folder on the server.

## What you need

**On your machine (to build):**

- Git and a clone of this repository
- [Node.js](https://nodejs.org/) 24 and [pnpm](https://pnpm.io/) 11
- Docker with Buildx (`docker buildx version`)

**On the server (to run):**

- Docker Engine 20.10+ and the Compose plugin (`docker compose version`)

## 1. Find the server's CPU type

The image must match the server's CPU. On the server, run:

```bash
uname -m
```

| Output | Platform to build |
| --- | --- |
| `x86_64` | `linux/amd64` (the default) |
| `aarch64` / `arm64` | `linux/arm64` |

## 2. Build the image on your machine

From the repository root:

```bash
scripts/build-image.sh                 # x86_64 server
PLATFORM=linux/arm64 scripts/build-image.sh   # ARM server
```

The script builds the web UI, then the image, tagged with a version (e.g. `roster-tracker:26.10`) and `latest`. The same script also gets the image to the server, in one of two ways (step 3).

## 3. Get the image to the server

Pick **one**.

### Option A: through a container registry (recommended)

Use any registry; these examples use GitHub Container Registry (`ghcr.io`), which is free for your GitHub account.

1. Create a GitHub personal access token (classic) with the `write:packages` scope, then log in **on your machine**:

   ```bash
   echo <token> | docker login ghcr.io -u <github-user> --password-stdin
   ```

2. Build and push:

   ```bash
   scripts/build-image.sh --push ghcr.io/<github-user>/roster-tracker
   ```

3. The package is private by default. Either make it public (GitHub → your profile → Packages → roster-tracker → Package settings → Change visibility), or log in **on the server** with a token that has `read:packages`:

   ```bash
   echo <token> | docker login ghcr.io -u <github-user> --password-stdin
   ```

In `.env` on the server, use `ROSTER_IMAGE=ghcr.io/<github-user>/roster-tracker:<version>`.

### Option B: copy the image file (no registry)

```bash
# On your machine
scripts/build-image.sh --save roster-tracker.tar.gz
scp roster-tracker.tar.gz user@<server-ip>:/tmp/

# On the server
gunzip -c /tmp/roster-tracker.tar.gz | docker load
```

In `.env` on the server, use `ROSTER_IMAGE=roster-tracker:<version>`.

## 4. Set up the server

On the server, create a folder with the two files:

```bash
mkdir -p ~/roster-tracker && cd ~/roster-tracker
```

Copy `deploy/docker-compose.yml` and `deploy/.env.example` from your machine:

```bash
# On your machine, from the repository root
scp deploy/docker-compose.yml user@<server-ip>:~/roster-tracker/
scp deploy/.env.example user@<server-ip>:~/roster-tracker/.env
```

Or download them on the server straight from GitHub:

```bash
curl -fsSLo docker-compose.yml https://raw.githubusercontent.com/anangnovriadi/roster-tracker/main/deploy/docker-compose.yml
curl -fsSLo .env https://raw.githubusercontent.com/anangnovriadi/roster-tracker/main/deploy/.env.example
```

Edit `.env`:

```bash
# Image from step 3
ROSTER_IMAGE=ghcr.io/<github-user>/roster-tracker:26.10

# Public address of the app. Without a domain, use the server IP and port.
# 203.0.113.10 is an example address; replace it with your server's IP.
ROSTER_INSTANCE_URL=http://203.0.113.10:5230

# Port on the server (the container always listens on 5230)
ROSTER_HOST_PORT=5230

# Database and uploads, relative to this folder
ROSTER_DATA_DIR=./data

# Time zone for logs
TZ=Asia/Jakarta
```

The folder now holds only:

```
~/roster-tracker/
├── docker-compose.yml
├── .env
└── data/            (created on first start: database + uploads)
```

## 5. Start

```bash
docker compose up -d
docker compose ps
docker compose logs -f
```

Check it answers:

```bash
curl http://localhost:5230/healthz
```

Then open `http://203.0.113.10:5230`, replacing the example address with your server's IP.

The container starts as root only long enough to give the data folder to its unprivileged user (UID/GID 10001), then drops privileges.

## 6. First sign-in

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

## Updating

On your machine, build and ship a new image:

```bash
git pull
scripts/build-image.sh --push ghcr.io/<github-user>/roster-tracker   # Option A
scripts/build-image.sh --save roster-tracker.tar.gz                  # Option B (then scp + docker load)
```

On the server, set the new version in `ROSTER_IMAGE` (or keep `:latest`) and restart:

```bash
docker compose pull      # Option A only
docker compose up -d
```

Database migrations run automatically on start. Take a backup first (see below).

## Access by IP address (no domain)

A domain is not required. The examples use `203.0.113.10`, an address reserved for documentation; replace it with your server's public IP.

1. In `.env`: `ROSTER_INSTANCE_URL=http://203.0.113.10:5230` (or leave it empty).
2. Open the port in the server firewall, e.g. `sudo ufw allow 5230/tcp`. On a cloud server, also allow inbound TCP 5230 in the provider's firewall or security group.
3. Open `http://203.0.113.10:5230`.

Signing in works over plain HTTP: the session cookie is only marked `Secure` when the request arrives over HTTPS. Over plain HTTP, passwords and notes travel unencrypted. That is fine on a private network or VPN, but on the public internet prefer a domain with HTTPS. If the server's IP changes, update `ROSTER_INSTANCE_URL`; it is only used for links in emails and the sitemap.

## HTTPS with a reverse proxy

Put the container behind a reverse proxy that terminates TLS, and set `ROSTER_INSTANCE_URL` to the public `https://` address. Bind the container port to localhost so it is reachable only through the proxy: `ROSTER_HOST_PORT=127.0.0.1:5230`.

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

## Configuration reference

Every setting can go in `.env`; the compose file passes the whole file to the container. Server flags use the `ROSTER_` prefix (dashes become underscores):

| Variable | Description | Default |
| --- | --- | --- |
| `ROSTER_IMAGE` | Image to run (used by Compose) | — (required) |
| `ROSTER_HOST_PORT` | Port on the server (used by Compose) | `5230` |
| `ROSTER_DATA_DIR` | Data folder on the server (used by Compose) | `./data` |
| `ROSTER_INSTANCE_URL` | Public URL of the app (http or https, no query string) | empty |
| `ROSTER_DRIVER` | `sqlite`, `mysql` or `postgres` | `sqlite` |
| `ROSTER_DSN` | Database connection string (required for MySQL/PostgreSQL) | `roster_prod.db` in the data folder |
| `ROSTER_LOG_LEVEL` | `debug`, `info`, `warn` or `error` | `info` |
| `ROSTER_TRUSTED_PROXIES` | Proxies whose forwarding headers are trusted | `private` |
| `ROSTER_UID` / `ROSTER_GID` | User the server runs as inside the container | `10001` |
| `TZ` | Container time zone | `UTC` |

### Using MySQL or PostgreSQL

SQLite is the default and is enough for a small team. To use a database server, add to `.env`:

```bash
ROSTER_DRIVER=postgres
ROSTER_DSN=postgresql://user:password@db-host:5432/roster_tracker?sslmode=disable
```

Uploaded files still go to the data folder (or to S3 if you configure it), so keep the volume.

## Data and backups

Everything that matters is in the data folder (`ROSTER_DATA_DIR`):

| Path | Contents |
| --- | --- |
| `roster_prod.db` (+ `-wal`, `-shm`) | SQLite database: users, roster, notes, settings |
| `assets/` | Uploaded attachments |

Back up **both**. For a consistent SQLite copy, stop the container first:

```bash
cd ~/roster-tracker
docker compose stop
tar czf roster-backup-$(date +%F).tar.gz data
docker compose start
```

Restore by stopping the container, replacing `data/` with the backup, and starting it again.

Uploads have no size limit by default. To cap them, add a usage-limit deployment file to the data folder (`roster-tracker-instance-setting-usage-limits.json`).

## Starting over

To wipe all data and get a fresh admin account:

```bash
cd ~/roster-tracker
docker compose down
sudo rm -rf data
docker compose up -d
```

This deletes every member, note, roster day and upload.

## Building on the server instead

If you would rather build on the server, clone the repository there (with Node.js and pnpm installed) and use the build-from-source compose file:

```bash
cd web && pnpm install && pnpm release && cd ..
docker compose -f scripts/compose.yaml up -d --build
```

## Troubleshooting

| Problem | Fix |
| --- | --- |
| `exec format error` when the container starts | The image was built for a different CPU. Rebuild with the right `PLATFORM` (step 1). |
| `Set ROSTER_IMAGE in .env` | `.env` is missing or has no `ROSTER_IMAGE`; it must sit next to `docker-compose.yml`. |
| `pull access denied` / `unauthorized` | The registry package is private: log in on the server (Option A, step 3) or make the package public. |
| Build fails with `pattern dist/*: no matching files found` | The web UI was not built. `scripts/build-image.sh` does this; if you build by hand, run `cd web && pnpm release` first. |
| `permission denied` on the data folder | Make sure it exists; the entrypoint gives it to UID 10001. Under rootless Docker set `ROSTER_UID=0` and `ROSTER_GID=0` in `.env`. |
| Page loads but live updates stall behind a proxy | Disable proxy buffering for the app (see the Nginx example). |
| Visitors who are not signed in see a sign-in page | The instance access mode was changed to private; the seeded instance is public. |
