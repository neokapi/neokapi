#!/bin/sh
# Render config.toml from template using environment variables.
# Falls back to the template as-is if envsubst is not available.

set -e

if command -v envsubst >/dev/null 2>&1; then
    envsubst < /bravo/config.toml.template > /bravo/config.toml
else
    cp /bravo/config.toml.template /bravo/config.toml
fi

exec "$@"
