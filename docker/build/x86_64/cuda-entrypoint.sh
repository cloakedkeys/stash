#!/bin/sh
set -eu

# Patching the mounted NVIDIA driver libraries needs root, so skip it otherwise.
if [ "$(id -u)" -eq 0 ]; then
  exec /usr/local/bin/nvidia-patch-entrypoint.sh "$@"
else
  exec "$@"
fi
