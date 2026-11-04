#!/bin/sh
# Cron wrapper: load .env next to this script, then run.
cd "$(dirname "$0")" || exit 1
set -a
. ./.env
set +a
exec ./morningbrief "$@"
