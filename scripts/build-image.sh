#!/usr/bin/env bash
#
# Builds the Roster Tracker Docker image on this machine so the server only needs
# deploy/docker-compose.yml and a .env file. See DEPLOY-DOCKER.md.
#
# Usage:
#   scripts/build-image.sh                                   # build roster-tracker:latest
#   scripts/build-image.sh --push ghcr.io/<user>/roster-tracker
#   scripts/build-image.sh --save roster-tracker.tar.gz      # file to copy with scp
#
# Environment:
#   PLATFORM   target platform of the server (default linux/amd64; linux/arm64 for ARM servers)
#   VERSION    version label (default: derived from the latest commit, e.g. 26.10)

set -euo pipefail

cd "$(dirname "$0")/.."

PLATFORM="${PLATFORM:-linux/amd64}"
VERSION="${VERSION:-$(scripts/release_version.sh development-version)}"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
REPOSITORY="roster-tracker"
MODE="load"
SAVE_FILE=""

while [ "$#" -gt 0 ]; do
  case "$1" in
    --push)
      MODE="push"
      REPOSITORY="${2:?--push needs an image repository, e.g. ghcr.io/<user>/roster-tracker}"
      shift 2
      ;;
    --save)
      MODE="save"
      SAVE_FILE="${2:?--save needs a file name, e.g. roster-tracker.tar.gz}"
      shift 2
      ;;
    -h|--help)
      sed -n '2,15p' "$0"
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      exit 1
      ;;
  esac
done

echo "==> Building the web UI"
(cd web && pnpm install --frozen-lockfile && pnpm release)

echo "==> Building ${REPOSITORY}:${VERSION} for ${PLATFORM}"
BUILD_ARGS=(
  buildx build
  --platform "$PLATFORM"
  -f scripts/Dockerfile
  --build-arg "VERSION=${VERSION}"
  --build-arg "COMMIT=${COMMIT}"
  -t "${REPOSITORY}:${VERSION}"
  -t "${REPOSITORY}:latest"
)
if [ "$MODE" = "push" ]; then
  BUILD_ARGS+=(--push)
else
  BUILD_ARGS+=(--load)
fi
docker "${BUILD_ARGS[@]}" .

if [ "$MODE" = "save" ]; then
  echo "==> Saving to ${SAVE_FILE}"
  docker save "${REPOSITORY}:${VERSION}" "${REPOSITORY}:latest" | gzip > "$SAVE_FILE"
fi

echo
echo "Done. Image: ${REPOSITORY}:${VERSION} (also tagged :latest)"
case "$MODE" in
  push) echo "On the server, set ROSTER_IMAGE=${REPOSITORY}:${VERSION} in .env and run: docker compose pull && docker compose up -d" ;;
  save) echo "Copy ${SAVE_FILE} to the server, run: gunzip -c ${SAVE_FILE} | docker load, set ROSTER_IMAGE=${REPOSITORY}:${VERSION} and run: docker compose up -d" ;;
  load) echo "The image is in the local Docker. Use --push or --save to get it to the server." ;;
esac
