# 4. Plan B: a PC that only hosts (double-click zmp-server)

**English** | [Español](../es/04-host-on-your-pc.md)

Usually the host just presses **Host and play** in the game ([main guide](00-host-and-send-the-code.md)). Use the
server on its own when a PC should host **without playing**: an always-on home PC, a spare laptop, a mini PC. It is
the same `zmp-server` the game uses, with a small page of its own that shows the invite code.

The server never runs the game: it only orders and relays everyone's buttons and keeps the rooms' shared saves, so it
barely uses CPU, memory or bandwidth.

## 1. Start it

1. Take `zmp-server.exe` from the Windows zip (it is next to `soh.exe`), or download
   `zmp-server-<version>-windows-amd64.exe` from the release. Put it in **a folder of its own**, for example
   `C:\Games\ZMP-server`: it writes its files next to itself.
2. **Double-click it.** Two things open:
   - a console window: it is the server's log. Keep it open while you play; closing it stops the server;
   - a page in your browser, at `http://127.0.0.1:<port>/`, with the invite code. Only this PC can open that page.
     If the browser does not open, the console prints the page's address: open it by hand.
3. The first time, **Windows Firewall** asks whether to allow `zmp-server`: allow it (on private networks, and on
   public ones too if your home network is set as "Public").

The first start creates, next to the program:

- `zmp-server.toml`: its settings, with a **random room name** and a **random 8-character password**;
- `data\`: the rooms' shared saves and room keys.

Then it listens on every network card on port **47100**, asks your router to open that port (UPnP, NAT-PMP or PCP),
and announces its room on your local network (so it appears in the game's **Join > On your local network** list).

## 2. The page

| Part | What it does |
|---|---|
| **Invite code** | The code, with **Copy code** and **Copy link**, and the addresses inside it. Friends paste it in the game's Join tab. |
| **From the internet** | Whether friends on the internet can reach this PC, with advice; the same messages as in the game ([what they mean](00-host-and-send-the-code.md#what-the-from-the-internet-line-means)). |
| **Room and password** | **Room**: rename it (**Change**). **Password**: **Show**, **Change**, **New random**, **Remove**. |
| **The code carries** | **the password** (friends join without typing anything), **only a hint** (they type the password; the hint helps them), or **nothing** (they are asked for it). Press **Save**. Choose "only a hint" if you will post the code anywhere public. |
| **Players** | Who is in, their state and their ping. |
| **Stop** | Stops the server (it asks first). |

The page has a language selector (Español / English) at the top.

To play on that same PC too: open the game, and paste the code (or click the room in **On your local network**).

**Stopping**: press **Stop** on the page, press Ctrl+C in the console, or close the console. The server removes the
rule it asked the router for. Players are disconnected; the room's progress stays in `data\`.

This server hosts **only its own room**: anyone who asks for another room name gets "That server has no such room".

## 3. The settings file

`zmp-server.toml` is a text file; edit it with Notepad **while the server is stopped**. The page changes the room,
the password and what the code carries for you; the rest is here. An unknown key is reported in the log and ignored.

| Key | Default | What it does |
|---|---|---|
| `listen` | `":47100"` | Where to listen. `":47101"` for another port; `"127.0.0.1:47100"` for this PC only (no router). |
| `data` | `"data"` | The data folder, relative to the file. |
| `server_name` | empty | Name shown in the local network list. |
| `room`, `password` | random | The room and its password (empty password = no password). |
| `invite_secret`, `hint` | `"password"` | What the code carries: `"password"`, `"hint"` or `"none"`, and the hint text. |
| `port_mapping` | `true` | Ask the router to open the port. `false` if you open it by hand ([guide](09-open-the-port-by-hand.md)). |
| `public_address` | empty | A domain, a fixed public IP or a `wss://` address that goes **first** in the code. Use it when you forward the port by hand, have a domain, or put a [wss proxy](12-wss-for-browsers.md) in front. |
| `open_browser` | `true` | Open the page when it starts. |
| `ui_listen` | empty | Where the page is served; always on `127.0.0.1`. |
| `lan_announce` | `true` | Announce the room on your local network (UDP, port 47100, never beyond your network). |
| `lan_port`, `lan_address` | `47100`, empty | The UDP port of the announcement and, for tests only, one address to send it to. |

The server keys of the next section (`require_room_password`, `server_password`, `max_rooms`, `relay`...) also work
in this file.

## 4. Linux, a Raspberry Pi, or without a page

- On Linux (`zmp-server-<version>-linux-amd64`, or `-linux-arm64` for a Raspberry Pi 4/5), `zmp-server -app` does
  the same as the double click: the settings file next to the program, the router, the local network announcement
  and the page on `127.0.0.1`. Add `-no-browser` on a machine without a desktop (the console prints the page's
  address); `-config <file>` reads another settings file.
- With any option on the command line (for example `zmp-server.exe -listen :47100`), the server runs as a plain
  server: no settings file next to it, no router, no page, any room name. That is the mode for a VPS
  ([Docker guide](06-host-on-a-vps.md)), a [relay](11-relay.md) and the [browser page](07-browser-mode.md)
  (`-web`).

Options of the plain server. Each one can also come from an environment variable or from a file given with
`-config <file>` (the command line wins, then the variable, then the file):

| Option | Variable | File key | Default |
|---|---|---|---|
| `-listen` | `ZMP_LISTEN` | `listen` | `:47100` |
| `-data` | `ZMP_DATA` | `data` | `./data` |
| `-name` | `ZMP_SERVER_NAME` | `server_name` | empty |
| `-server-password-file` | `ZMP_SERVER_PASSWORD_FILE` or `ZMP_SERVER_PASSWORD` | `server_password_file` or `server_password` | no server password |
| `-require-room-password` | `ZMP_REQUIRE_ROOM_PASSWORD` | `require_room_password` | `false` |
| `-max-rooms` | `ZMP_MAX_ROOMS` | `max_rooms` | 0 = no limit |
| `-max-conns-per-ip` | `ZMP_MAX_CONNS_PER_IP` | `max_conns_per_ip` | 0 = no limit |
| `-max-rooms-per-ip` | `ZMP_MAX_ROOMS_PER_IP` | `max_rooms_per_ip` | 0 = no limit |
| `-list-rooms` | `ZMP_LIST_ROOMS` | `list_rooms` | `false` |
| `-room-key-ttl` | `ZMP_ROOM_KEY_TTL` | `room_key_ttl` (`30m`, `12h`, `7d`) | never |
| `-relay` | `ZMP_RELAY` | `relay` | `false` ([relay guide](11-relay.md)) |
| `-web <folder>` | | | serve the [browser version](07-browser-mode.md) on the same port |
| `-version` | | | print the version and the protocol number |

On a plain server, the first player who enters a room with a password sets that room's password; everyone after
needs it.

## 5. Passwords and safety

- Room passwords never travel in clear: the game proves it knows the password, and the server keeps only a key
  derived from it (in `data\room_keys.json`).
- A **server password** (`server_password` in the file, or `ZMP_SERVER_PASSWORD`; never on the command line, where
  other programs could read it) closes the whole server: players type it in the game's **Advanced > Server password
  (if it has one)**.
- After 5 wrong passwords in a minute from one address, that address waits 60 seconds ("Too many failed attempts").
- Older games (ZMP 0.6.x, protocol 4) can still enter rooms **without** a password on this server, if they have the
  same version as the room. They cannot enter rooms with a password or servers with a server password.
- Only one TCP port is used (47100 by default) for the game, the browser version and the health check
  `http://<address>:47100/healthz`.

## How many players and how far apart

Up to **6 players** per room. Lockstep waits for the slowest connection, so the game feels best when everyone is in
the same region (same country or neighbouring ones). The server adapts the input delay to the connections every few
seconds.
