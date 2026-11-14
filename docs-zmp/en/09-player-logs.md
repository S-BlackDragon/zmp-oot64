# 9. Player logs on the server (optional, off unless the server turns it on)

A room server can keep its players' logs, so that when something goes wrong on one PC (a desync, a crash, a player
who could not join) the person who runs the server can read what happened on every PC afterwards, without asking
everybody to find and send their `logs` folder.

**It is off by default.** A `zmp-server` collects logs only when its owner starts it with a log folder (below). The
game then sends its logs **only to that server**, and only after the server has said, when you connect, that it
collects them. A server started without a log folder never receives anything: the game does not try.

## For players

### How you know

When you connect, the game writes one of these lines in `logs\zmp.log`:

- `logship: this server does not collect logs: nothing is sent to it`
- `logship: the server collects logs: this run's logs go to it every 180 s ...`
- `logship: the server collects logs, but the setting gZmp.Logs.Upload is off: nothing is sent`

### What is sent

- Desktop: `logs\zmp.log` (ZMP's own lines) and `logs\Ship of Harkinian.log` (the game's log), only the part written
  by the current run of the game; the file `logs\desync-<tick>.txt` of a desync, if your game wrote one; and, after a
  crash, the part of the previous run's logs that had not been sent yet.
- Browser: the ZMP log of that tab.
- Each upload carries only what the server does not have yet (appends), at most 2 MB at a time.
- Never sent: your ROM or anything made from it, your saves, screenshots, settings or any other file.

### When

On connecting (what is still pending), every 3 minutes while you are connected, at the end of a session, on a desync,
and after a crash (from the crash handler, and again on the next start for whatever did not go out). The upload runs on
its own thread (in the browser, a background request): the game never waits for it, and a failed upload is only
noted in your log and tried again later.

### Where

To the same address and port you connected to, as an HTTP `POST` to `/zmp/logs` on that port, with a token the server
gave this connection. Over plain `http` (the desktop game, or a browser page connected with `ws://`) the logs travel
unencrypted, like the game's own traffic; with `wss://` they go over HTTPS.

### Not sending them

Untick **"Enviar mis registros al servidor si los recoge (para que puedan ver los fallos)"** (send my logs to the
server if it collects them) in the room window, under the connection fields. It takes effect at once and is
remembered (`gZmp.Logs.Upload` = 0 in `shipofharkinian.json`). The browser version has the same checkbox in the
game's room window.

### Privacy

The logs contain your player name, the room name, the server address, details of your PC such as the graphics card, and
file paths that can include your Windows user name. The server also notes the IP address you connected from in the
room's event log. Only the person who runs the server (and whoever can read that PC's or VPS's disk) can read them.
Leave the checkbox ticked only on servers run by someone you trust.

## For server owners

### Turning it on

Give the server a folder for the logs:

```powershell
zmp-server.exe -listen :47100 -data C:\Games\ZMP-server\data -log-dir C:\Games\ZMP-server\logs
```

or set the environment variable `ZMP_LOG_DIR` to that folder. The server log then says
`zmp logs: ON, store <folder> (...)`. Without `-log-dir` (and without `ZMP_LOG_DIR`) collection is off, the server
does not announce it, and an upload to `/zmp/logs` is answered `404`.

With Docker (`server/deploy/docker-compose.yml`), uncomment the marked `./logs:/logs` volume and the `ZMP_LOG_DIR`
line, create the folder (`mkdir -p logs && sudo chown 1000:1000 logs`) and run `docker compose up -d`.

To turn it off, start the server again without the option. To delete everything, delete the folder.

**Tell your players** that your server keeps their logs, keep them only as long as you need them, and do not publish
them (see Privacy above).

### Limits

| Option (environment variable) | Default | What |
|---|---|---|
| `-log-max-total` (`ZMP_LOG_MAX_TOTAL`) | `2G` | size of the whole folder; past it the oldest files are deleted until it is at 90 % |
| `-log-max-upload` (`ZMP_LOG_MAX_UPLOAD`) | `8M` | largest single upload |
| `-log-max-per-day` (`ZMP_LOG_MAX_PER_DAY`) | `128M` | what one player name may upload in a UTC day |

All the players together may upload at most 1 GiB per UTC day. Sizes take a number of bytes or `512M`, `2G`.

### What is in the folder

```
sessions/<room>/<session>/server-events.log               joins, leaves and desyncs of that room session
sessions/<room>/<session>/p<id>-<name>/<run>-<kind>.log   one player's log of one run of the game
server/zmp-server-<yyyymmdd>.log                          a copy of the server's own log, one file per day
```

The server names every file itself, from what it knows about the connection (room, session, player id and name),
never from what the client sends, and every name is sanitized. A token is valid for one connection's player and
expires after 72 hours.
