#!/bin/sh
# Docker may create a missing bind mount (e.g. ./data) owned by root, while the
# app runs as UID 10001. Fix only the top-level directories we own, then drop
# privileges. Existing user files are never recursively chowned.
set -e
if [ "$(id -u)" = "0" ]; then
  chown 10001:10001 /data 2>/dev/null || true
  mkdir -p /data/files /data/tmp
  chown 10001:10001 /data/files /data/tmp 2>/dev/null || true
  exec su-exec 10001:10001 /usr/local/bin/private-drop "$@"
fi
exec /usr/local/bin/private-drop "$@"
