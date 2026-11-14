# 6. Host on a VPS with Docker (always on)

**English** | [Español](../es/06-host-on-a-vps.md)

A small rented Linux server (a VPS) keeps the room server on all the time, so the group can play without anyone's
PC hosting. `zmp-server` needs very little: the smallest VPS of any provider is enough (it never runs the game).
Pick one in the same region as your group.

## 1. Prepare the VPS

You need a Linux VPS (Debian or Ubuntu are the easiest) with Docker. On a fresh Debian/Ubuntu VPS, logged in by SSH:

```bash
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker $USER     # then log out and back in
```

Open **TCP port 47100** in the provider's firewall (the "security group" or "firewall" page of the VPS) and, if the
VPS has its own firewall (`ufw`), there too: `sudo ufw allow 47100/tcp`.

## 2. Get the server files

```bash
git clone -b zmp --depth 1 https://github.com/S-BlackDragon/zmp-oot64.git
cd zmp-oot64/server/deploy
mkdir -p data && sudo chown 1000:1000 data    # the container writes the saves as user 1000
docker compose up -d --build                  # builds the image from source (about a minute) and starts it
docker compose logs -f zmp-server             # Ctrl+C to stop watching; the server keeps running
```

You should see `listening on [::]:47100 (tcp and websocket)`. Check from your own PC: open
`http://<VPS address>:47100/healthz` in a browser; it answers `zmp-server <version> protocol 4`.

Friends type the VPS address (its IP or a domain name pointing to it), port `47100`, and the room name.

The container is locked down: it runs as a normal user, its filesystem is read-only except `data/`, it has no extra
privileges, and it is limited to 512 MB of memory and one CPU. It restarts by itself if the VPS reboots.

Useful commands (from `zmp-oot64/server/deploy`):

```bash
docker compose ps                   # is it running?
docker compose logs --tail 100      # recent log
docker compose restart              # restart (players reconnect by themselves)
docker compose down                 # stop
git pull && docker compose up -d --build   # update to the latest version
```

The rooms' shared saves are in `server/deploy/data/`; back that folder up if you care about them.

## 3. Without Docker

Copy `zmp-server-<version>-linux-amd64` (or `-linux-arm64`) from a release to the VPS and run it with
`./zmp-server-<version>-linux-amd64 -listen :47100 -data ./data`. To keep it running after you log out, use a systemd unit,
`tmux`, or `screen`.

## 4. Browser mode on the VPS

To let friends play in the browser, put the browser build (from `ZMP-<version>-web.zip`, or built yourself) in
`server/deploy/web/` and uncomment the two marked lines in `server/deploy/docker-compose.yml` (the `./web:/web:ro`
volume and the `command` with `-web /web`), then `docker compose up -d`. Friends open `http://<VPS address>:47100/`.

That page is plain `http`. It works, but some browsers warn about it, and a page opened over `https` can only connect
with `wss://`. For a clean `https://` address, put a TLS reverse proxy in front, for example
[Caddy](https://caddyserver.com/) with a domain name pointing to the VPS. Add this service to the same
`docker-compose.yml`:

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
play.example.org {
    reverse_proxy zmp-server:47100
}
```

Open ports 80 and 443 in the firewall, then `docker compose up -d`. Caddy gets a free certificate by itself. Browser
players open `https://play.example.org/`; the page fills in `wss://play.example.org/` as the server. Desktop
players keep using the address and port `47100` directly (the desktop game speaks plain TCP, which does not go
through the proxy).

The Caddy setup above is a standard example that the project has not tested end to end; the plain `http` setup
(`-web` on port 47100) is the one covered by the server's tests.
