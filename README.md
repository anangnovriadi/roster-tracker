# Roster Tracker

Self-hosted web app with a Go backend and a React frontend, served from a single binary.

## Tech Stack

**Backend**

- Go 1.27.2
- Echo v5 (HTTP server)
- Connect RPC and gRPC-Gateway (API)
- Protocol Buffers with [buf](https://buf.build) for code generation
- Databases: SQLite (default), MySQL, or PostgreSQL

**Frontend**

- React 19 and TypeScript 7
- Vite 8
- Tailwind CSS v4
- React Query v5
- Base UI components
- Biome (lint/format) and Vitest (tests)

## Prerequisites

- [Go](https://go.dev/dl/) 1.27.2
- [Node.js](https://nodejs.org/) 24
- [pnpm](https://pnpm.io/) 11 (`npm install -g pnpm`)
- Optional: [buf](https://buf.build/docs/installation) if you change `.proto` files
- Optional: [golangci-lint](https://golangci-lint.run/) v2.14.0 for Go linting

## Setup

```bash
git clone <repository-url> roster-tracker
cd roster-tracker

# Install frontend dependencies
cd web && pnpm install && cd ..

# Download Go modules
go mod download
```

## Running Locally

Run all commands from the repository root.

### Option 1: Development mode (hot reload)

Use two terminals.

```bash
# Terminal 1: backend API on http://localhost:8081
go run -buildvcs=true ./cmd/roster-tracker --port 8081
```

```bash
# Terminal 2: frontend dev server on http://localhost:3001
cd web && pnpm dev
```

Open http://localhost:3001. The dev server proxies API requests to the backend on port 8081, and UI changes reload instantly.

### Option 2: Single server (production-like)

Build the frontend into the Go binary, then run only the backend:

```bash
cd web && pnpm release && cd ..
go run -buildvcs=true ./cmd/roster-tracker --port 8081
```

Open http://localhost:8081. Run `pnpm release` again after any UI change.

### Admin account seeder

On every start, the server checks the database. If it has no users yet, a seeder creates this admin account (see `cmd/roster-tracker/defaults.go`):

| Field | Value |
| --- | --- |
| Username | `admin` |
| Full name | `Administrator` |
| Email | `admin@mail.com` |
| Password | `12345678` |

The seeder does nothing once any user exists, so restarting never resets or duplicates the admin.

### Signing in as admin

1. Start the app (Option 1 or Option 2 above).
2. Open the app and click **Sign in**.
3. Enter username `admin` (or email `admin@mail.com`) and password `12345678`.
4. Change the password right away in Settings → My Account.

The sign-in form accepts either a username or an email address in the same field:

- Usernames match exactly (letter case matters).
- Email addresses match case-insensitively, so `Admin@Mail.com` also works. Email sign-in only works for members who have an email set in their profile.
- If a legacy username contains `@` and equals another member's email, the username wins.

Self sign-up is disabled; the admin adds members in Settings → Member. Anyone can read notes and the duty roster without signing in.

### Starting over with a clean database

To wipe all data and get a fresh admin account from the seeder:

```bash
# 1. Stop the backend (Ctrl+C in its terminal)

# 2. Delete the SQLite database and uploaded files from the data directory
#    (the repository root by default, or the folder passed to --data)
rm -f roster_prod.db roster_prod.db-shm roster_prod.db-wal
rm -rf assets

# 3. Start the backend again; the seeder recreates the admin account
go run -buildvcs=true ./cmd/roster-tracker --port 8081
```

Then sign in again with `admin` (or `admin@mail.com`) / `12345678`. This deletes every note, member, roster entry, and upload, so back up `roster_prod.db` first if you might need the data. For MySQL or PostgreSQL, drop and recreate the database instead of deleting files.

### Data and configuration

By default, the SQLite database (`roster_prod.db`) and uploaded files are stored in the current directory. Useful flags:

| Flag | Description | Default |
| --- | --- | --- |
| `--port` | HTTP port | `8081` |
| `--data` | Data directory for the database and uploads | current directory |
| `--driver` | Database driver: `sqlite`, `mysql`, `postgres` | `sqlite` |
| `--dsn` | Database connection string (required for MySQL/PostgreSQL) | |
| `--instance-url` | Public URL of the instance | |
| `--log-level` | `debug`, `info`, `warn`, `error` | `info` |

Every flag can also be set as an environment variable with the `ROSTER_` prefix, for example `ROSTER_PORT=8081` or `ROSTER_DATA=./data`.

Example with PostgreSQL:

```bash
go run -buildvcs=true ./cmd/roster-tracker --port 8081 \
  --driver postgres \
  --dsn "postgresql://user:password@localhost:5432/roster_tracker?sslmode=disable"
```

## Testing and Linting

```bash
# Backend
go test ./...
golangci-lint run

# Frontend
cd web && pnpm lint && pnpm test

# Protocol Buffers (after editing .proto files)
cd proto && buf generate && buf lint
```

## Building

```bash
# Build the frontend into server/frontend/dist, then build the binary
cd web && pnpm release && cd ..
go build -o build/roster-tracker ./cmd/roster-tracker

./build/roster-tracker --port 8081
```

## License

[MIT](LICENSE)
