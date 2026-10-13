#!/usr/bin/env sh

# Fix ownership of the data directory (e.g. for users upgrading from older
# versions where files were created as root) and drop to a non-root user.
ROSTER_UID=${ROSTER_UID:-10001}
ROSTER_GID=${ROSTER_GID:-10001}
DATA_DIR="/var/opt/roster-tracker"

# ROSTER_ENTRYPOINT_SWITCHED marks that the privilege drop below has already run.
# su-exec preserves the environment, so the marker survives the re-exec. Without
# it, a target of UID 0 (e.g. ROSTER_UID=0, common under rootless Docker) would
# stay root after su-exec, re-enter this block, and loop forever.
if [ "$(id -u)" = "0" ] && [ -z "${ROSTER_ENTRYPOINT_SWITCHED:-}" ]; then
    # Started as root: fix permissions, then re-exec as the target user.
    if [ -d "$DATA_DIR" ]; then
        chown -R "$ROSTER_UID:$ROSTER_GID" "$DATA_DIR" 2>/dev/null || true
    fi
    echo "roster-tracker: starting as UID:GID ${ROSTER_UID}:${ROSTER_GID}"
    export ROSTER_ENTRYPOINT_SWITCHED=1
    exec su-exec "$ROSTER_UID:$ROSTER_GID" "$0" "$@"
fi
unset ROSTER_ENTRYPOINT_SWITCHED

file_env() {
   var="$1"
   fileVar="${var}_FILE"

   val_var="$(printenv "$var")"
   val_fileVar="$(printenv "$fileVar")"

   if [ -n "$val_var" ] && [ -n "$val_fileVar" ]; then
      echo "error: both $var and $fileVar are set (but are exclusive)" >&2
      exit 1
   fi

   if [ -n "$val_var" ]; then
      val="$val_var"
   elif [ -n "$val_fileVar" ]; then
      if [ ! -r "$val_fileVar" ]; then
         echo "error: file '$val_fileVar' does not exist or is not readable" >&2
         exit 1
      fi
      val="$(cat "$val_fileVar")"
   fi

   export "$var"="$val"
   unset "$fileVar"
}

file_env "ROSTER_DSN"

exec "$@"
