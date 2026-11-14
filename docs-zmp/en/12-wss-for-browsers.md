# 12. wss:// for browsers (secure connections)

**English** | [Español](../es/12-wss-for-browsers.md)

Browsers protect pages opened over `https://`: such a page may only open **secure** WebSocket connections, `wss://`.
So if your browser players open the game from an `https://` address, the room server must be reachable at a
`wss://` address. (A page opened over plain `http://`, for example straight from `zmp-server -web`, can use any
address, and needs none of this.)

`zmp-server` itself speaks plain TCP and plain WebSocket on one port. For `wss://`, put a **TLS proxy** in front of
it: a web server with a certificate that passes the connections on to `zmp-server`. You need a domain name pointing to
the server's public address.

## With Caddy and Docker (on a VPS)

[Caddy](https://caddyserver.com/) gets a free certificate by itself. Next to the server of the
[VPS guide](06-host-on-a-vps.md), add this service to `server/deploy/docker-compose.yml`:

```yaml
  caddy:
    image: caddy:2
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - ./caddy-data:/data
```

and create `server/deploy/Caddyfile` with your domain:

```
zmp.example.org {
    reverse_proxy zmp-server:47100
}
```

Open ports 80 and 443 in the firewall, then `docker compose up -d`. If the server also serves the browser version
(`-web`, [VPS guide](06-host-on-a-vps.md) section 6), browser players open `https://zmp.example.org/` and the page
fills in `wss://zmp.example.org/` as the server by itself.

This Caddy setup is a standard example that the project has not tested end to end; the plain `http` setup (`-web` on
port 47100) is the one covered by the server's tests.

## Who uses which address

- **Browser players**: `wss://zmp.example.org/`.
- **Desktop players** can use the same `wss://zmp.example.org/` address (the desktop game speaks `ws://` and `wss://`
  too), or the plain address and port 47100 if that port is open.
- An **invite code** can carry a `wss://` address: with `zmp-invite encode -addr wss://zmp.example.org/ ...`
  ([relay guide](11-relay.md)), or with `public_address = "wss://zmp.example.org/"` in the settings of the
  [double-click server](04-host-on-your-pc.md).

## Certificates

The certificate must be valid for that name and trusted by ordinary browsers (Caddy's free certificates are). With an
expired, self-made or wrong certificate, browsers refuse to connect and the desktop game says "The server's
certificate is not valid: not connecting (wss)". The game never falls back to an insecure connection.

## Behind a proxy or tunnel

All players reach `zmp-server` through the proxy, so they seem to come from one address. On a [relay](11-relay.md),
turn the per-address limits off (`-max-conns-per-ip 0 -max-rooms-per-ip 0`).
