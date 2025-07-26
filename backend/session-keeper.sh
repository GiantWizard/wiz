[unix_http_server]
file=/var/run/supervisor.sock
chmod=0700

[supervisord]
logfile=/var/log/supervisor/supervisord.log
pidfile=/var/run/supervisord.pid
childlogdir=/var/log/supervisor
nodaemon=true
minfds=1024
minprocs=200

[rpcinterface:supervisor]
supervisor.rpcinterface_factory = supervisor.rpcinterface:make_main_rpcinterface

[supervisorctl]
serverurl=unix:///var/run/supervisor.sock

; -------------------------------------------------------------------
; MEGA Session Manager - The single source of truth for the MEGA session
; -------------------------------------------------------------------
[program:mega-session-manager]
command=/usr/local/bin/mega-session-manager.sh
user=appuser
autostart=true
autorestart=true
stdout_logfile=/dev/stdout
stdout_logfile_maxbytes=0
stderr_logfile=/dev/stderr
stderr_logfile_maxbytes=0
environment=HOME="/home/appuser",PATH="/usr/local/bin:/usr/bin:/bin",MEGA_EMAIL="%(ENV_MEGA_EMAIL)s",MEGA_PWD="%(ENV_MEGA_PWD)s",MEGA_CMD_SOCKET="/home/appuser/.megaCmd/megacmd.sock"

; -------------------------------------------------------------------
; Metrics generator
; -------------------------------------------------------------------
[program:metrics-generator]
command=/app/metrics_generator
user=appuser
autostart=true
autorestart=true
stdout_logfile=/dev/stdout
stdout_logfile_maxbytes=0
stderr_logfile=/dev/stderr
stderr_logfile_maxbytes=0
environment=HOME="/home/appuser",PATH="/usr/local/bin:/usr/bin:/bin:/app",MEGA_CMD_SOCKET="/home/appuser/.megaCmd/megacmd.sock"

; -------------------------------------------------------------------
; Calculation engine - Relies on the managed session
; -------------------------------------------------------------------
[program:calculation-engine]
command=/app/calculation_service
user=appuser
autostart=true
autorestart=true
stdout_logfile=/dev/stdout
stdout_logfile_maxbytes=0
stderr_logfile=/dev/stderr
stderr_logfile_maxbytes=0
environment=HOME="/home/appuser",PATH="/usr/local/bin:/usr/bin:/bin:/app",PORT="9000",LOG_LEVEL="debug",MEGA_CMD_SOCKET="/home/appuser/.megaCmd/megacmd.sock"

; -------------------------------------------------------------------
; Your Go Application
; -------------------------------------------------------------------
[program:go-app]
command=/app/your-go-app-binary # Make sure to use the correct binary name
user=appuser
autostart=true
autorestart=true
stdout_logfile=/dev/stdout
stdout_logfile_maxbytes=0
stderr_logfile=/dev/stderr
stderr_logfile_maxbytes=0
environment=HOME="/home/appuser",PATH="/usr/local/bin:/usr/bin:/bin:/app",PORT="8080",MEGA_CMD_SOCKET="/home/appuser/.megaCmd/megacmd.sock"