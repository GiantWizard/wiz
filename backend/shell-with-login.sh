#!/usr/bin/env bash
set -euo pipefail

# point the CLI at the background server
export MEGA_CMD_SOCKET="/home/appuser/.megaCmd/megacmdserver.sock"

# first, make sure we're logged in
mega-login "$MEGA_EMAIL" "$MEGA_PWD" < /dev/null

# now spawn the interactive shell, cd and list
exec mega-cmd <<'EOF'
cd /remote_metrics
ls
EOF
