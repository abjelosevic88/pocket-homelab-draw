# Reverse proxy

Set `BASE_URL` to the public address and `SECURE_COOKIES=true` behind HTTPS. See [deploy/reverse-proxy/Caddyfile](../deploy/reverse-proxy/Caddyfile). `/api/*` is reachable for Homepage widgets, shortcuts and linked apps; they send their own bearer token.

Live sessions on shared sketches are a WebSocket at `/live/{id}` on the same address. Caddy and Traefik pass WebSockets through as they are; with nginx add `proxy_http_version 1.1; proxy_set_header Upgrade $http_upgrade; proxy_set_header Connection "upgrade";` and a long `proxy_read_timeout`. Keep the `Host` header: the server only accepts a live session opened from its own address.
