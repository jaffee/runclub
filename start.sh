#!/bin/sh
# Start the application in the background
/usr/local/bin/runclub --port 9000 &
# Start Caddy in the foreground
caddy run --config /etc/caddy/Caddyfile --adapter caddyfile