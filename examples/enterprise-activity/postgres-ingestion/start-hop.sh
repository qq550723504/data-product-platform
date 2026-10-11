#!/bin/sh
set -eu
# Secrets are resolved only inside the authorized execution process. No set -x.
SOURCE_DB_PASSWORD="$(cat /run/secrets/source-password)"
# This trial creates hex secrets only; reject other encodings before JSON.
case "$SOURCE_DB_PASSWORD" in ''|*[!0-9a-f]*) exit 1;; esac
export HOP_CONFIG_FOLDER=/tmp/hop-config
export HOP_AUDIT_FOLDER=/tmp/hop-audit
export HOP_METADATA_FOLDER=/metadata
mkdir -p "$HOP_CONFIG_FOLDER" "$HOP_AUDIT_FOLDER"
umask 077
# Hop variables inherit JVM/config properties, not arbitrary shell env vars.
# Resolve the secret into executor-private tmpfs, never into submitted metadata,
# parameters, process arguments, the host repository or archived evidence.
printf '{"config":{"variables":[{"name":"SOURCE_DB_PASSWORD","value":"%s","description":"executor-private synthetic secret"}]}}\n' "$SOURCE_DB_PASSWORD" > "$HOP_CONFIG_FOLDER/hop-config.json"
unset SOURCE_DB_PASSWORD
exec /opt/hop/hop-server.sh /run/secrets/hop-server.xml
