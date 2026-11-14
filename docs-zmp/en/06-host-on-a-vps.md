# 6. Plan B: a VPS with Docker (always on)

**English** | [Español](../es/06-host-on-a-vps.md)

A small rented Linux server (a VPS) keeps the room server on all the time, so the group can play without anyone's
PC hosting, and it is reachable from anywhere (no router, no CGNAT problems). `zmp-server` needs very little: the
smallest VPS of any provider is enough (it never runs the game). Pick one in the same region as your group.

## 1. Prepare the VPS

You need a Linux VPS (Debian or Ubuntu are the easiest) with Docker. On a fresh Debian/Ubuntu VPS, logged in by SSH:

```bash
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker $USER     # then log out and back in
```

Open **TCP port 47100** in the provider's firewall (the "security group" or "firewall" page of the VPS) and, if the
VPS has its own firewall (`ufw`), there too: `sudo ufw allow 47100/tcp`.

## 2. Start the server

```bash
git clone -b zmp --depth 1 https://github.com/S-BlackDragon/zmp-oot64.git
cd zmp-oot64/server/deploy
mkdir -p data && sudo chown 1000:1000 data    # the container writes the saves as user 1000
docker compose up -d --build                  # builds the image from source (about a minute) and starts it
docker compose logs -f zmp-server             # Ctrl+C to stop watching; the server keeps running
```

You should see `listening on [::]:47100 (tcp and websocket)`. Check from your own PC: open
`http://<VPS address>:47100/healthz` in a browser; it answers `zmp-server <version> protocol 5`.

The container is locked down: it runs as a normal user, its filesystem is read-only except `data/`, it has no extra
privileges, and it is limited to 512 MB of memory and one CPU. It restarts by itself if the VPS reboots.

## 3. Rooms, passwords and the invite code

This is a plain server: any room name works, and the **first player who enters a room with a password sets it**. So
the host of the group:

1. in the game, **Join > By address**: **Server** = the VPS address (its IP or a domain name pointing to it),
   **Room** = a name that is not easy to guess, **Password** = a password; **Connect**;
2. gives the friends the same three things, or an **invite code** made with `zmp-invite` (in the Windows zip, and in
   the release for Linux):

```
zmp-invite encode -addr 203.0.113.10 -room forest-k3f9 -password make-one-up
```

It prints the code (`ZMP-...`) and the `zmp://` link. Add `-port <port>` if the server is not on 47100, or use
`-hint "..."` instead of `-password` to put only a hint in the code. `zmp-invite decode <code>` shows what a code
carries.

Server settings go in the `environment:` part of the `zmp-server` service in `server/deploy/docker-compose.yml`
(all of them are in the [options table](04-host-on-your-pc.md#4-linux-a-raspberry-pi-or-without-a-page)), for
example:

```yaml
    environment:
      ZMP_SERVER_NAME: "Our server"
      ZMP_REQUIRE_ROOM_PASSWORD: "true"     # no room without a password
```

then `docker compose up -d`. To let strangers make rooms too (a public relay), see [Relay](11-relay.md).

## 4. Useful commands

From `zmp-oot64/server/deploy`:

```bash
docker compose ps                   # is it running?
docker compose logs --tail 100      # recent log
docker compose restart              # restart (players reconnect by themselves)
docker compose down                 # stop
git pull && docker compose up -d --build   # update to the latest version
```

The rooms' shared saves and room keys are in `server/deploy/data/`; back that folder up if you care about them.

## 5. Without Docker

Copy `zmp-server-<version>-linux-amd64` (or `-linux-arm64`) from a release to the VPS and run it with
`./zmp-server-<version>-linux-amd64 -listen :47100 -data ./data`. To keep it running after you log out, use a systemd
unit, `tmux` or `screen`.

## 6. Browser players on the VPS

To serve the browser version from the VPS, put the browser build (from `ZMP-<version>-web.zip`, or built yourself)
in `server/deploy/web/` and uncomment the two marked lines in `server/deploy/docker-compose.yml` (the `./web:/web:ro`
volume and the `command` with `-web /web`), then `docker compose up -d`. Friends open `http://<VPS address>:47100/`.

That page is plain `http`. It works, but some browsers warn about it, and a page opened over `https` can only connect
with `wss://`. For a clean `https://` address, put a TLS proxy in front: see [wss:// for browsers](12-wss-for-browsers.md).
