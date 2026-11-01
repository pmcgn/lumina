#!/bin/sh
# Container entrypoint: make sure a config exists, then start lumina.
# /config is usually a mount that hides anything baked into the image, so the
# default is copied there on first start. An existing file is never touched.
set -eu

cfg="${LUMINA_CONFIG:-/config/config.yaml}"
default="${LUMINA_DEFAULT_CONFIG:-/usr/share/lumina/config.yaml}"

if [ ! -e "$cfg" ] && [ -f "$default" ]; then
  if mkdir -p "$(dirname "$cfg")" && cp "$default" "$cfg"; then
    echo "no configuration found, wrote the default to $cfg"
  else
    echo "warning: cannot write the default configuration to $cfg (is the mount writable for uid $(id -u)?)" >&2
  fi
fi

exec /lumina "$@"
