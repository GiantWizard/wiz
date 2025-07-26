#!/usr/bin/env bash
set -eo pipefail
export MEGA_CMD_SOCKET="/home/appuser/.megaCmd/megacmdserver.sock"
# login once
mega-login "$MEGA_EMAIL" "$MEGA_PWD" < /dev/null

while true; do
  mega-ls --non-interactive -q /remote_metrics
  sleep 60
done
