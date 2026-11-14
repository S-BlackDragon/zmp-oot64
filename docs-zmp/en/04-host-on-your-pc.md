# 4. Host on your own PC (port forwarding)

**English** | [Español](../es/04-host-on-your-pc.md)

One person in the group runs the room server, `zmp-server`. It is a single small program: it does not run the game,
it only orders and relays everyone's inputs and keeps the rooms' saves, so it barely uses CPU, memory or bandwidth.
The host can play on the same PC at the same time.

If you cannot (or do not want to) change your router's settings, use [Tailscale or ZeroTier](05-host-with-a-virtual-lan.md)
instead: it is usually easier.

## 1. Run the server

1. Put `zmp-server-<version>-windows-amd64.exe` in a folder of its own, for example `C:\Games\ZMP-server`, and rename
   it `zmp-server.exe` if you like.
2. Double-click it. A console window opens and shows a line like
   `zmp-server 0.6.2 protocol 4 listening on [::]:47100 (tcp and websocket)`. Keep that window open while you play;
   closing it stops the server.
3. The first time, **Windows Firewall** asks whether to allow it: allow it on **Private networks** (and on Public
   networks only if your home network is marked as public).

The server writes the rooms' shared saves in a `data` folder next to it. To use another port or folder, start it from
a terminal: `zmp-server.exe -listen :47100 -data C:\Games\ZMP-server\data`. `zmp-server.exe -version` prints its
version.

On Linux or a Raspberry Pi: `./zmp-server-<version>-linux-amd64` (or `-linux-arm64`), same options. To keep it running in the
background, see [Host on a VPS](06-host-on-a-vps.md).

## 2. Who connects to what

| Who | Server address to type |
|---|---|
| You, on the PC that runs the server | `127.0.0.1` |
| Someone on your home network (same router) | your PC's local address, for example `192.168.0.20` (see below) |
| Friends over the internet | your **public** address, after step 3 |

Port: `47100` in every case. Your local address: open a terminal (Win+R, `cmd`) and run `ipconfig`; it is the
"IPv4 Address" of your network adapter.

## 3. Open the port on your router

Friends over the internet can only reach your PC if your router forwards the port to it:

1. Give your PC a fixed local address: in the router, find the "DHCP reservation" / "static lease" option and reserve
   the current local address for your PC (otherwise it may change and the forwarding stops working).
2. Find **Port forwarding** (also called "Virtual server", "NAT", or "Port mapping") in the router's settings
   (usually at `http://192.168.0.1` or `http://192.168.1.1`; the address and password are often on a sticker on the
   router).
3. Add a rule: **TCP**, external port **47100**, internal port **47100**, to your PC's local address.
4. Find your public address (search "what is my IP" in a browser) and send it to your friends with the port and the
   room name.
5. Test it: ask a friend to connect. (Testing from your own PC with your public address may not work on some
   routers; that does not mean it is broken for others.)

If it does not work:

- If the router's "WAN" or "Internet" address is different from the public address the browser shows, or starts with
  `100.64` to `100.127`, your internet provider uses **CGNAT** and port forwarding cannot work. Use
  [Tailscale or ZeroTier](05-host-with-a-virtual-lan.md), or a [VPS](06-host-on-a-vps.md).
- Check that the server window is still open and that the Windows Firewall allowed it.

## 4. Safety

- The server has no accounts or passwords: **anyone who knows your address, the port and the room name can join that
  room**. Use a room name that is not easy to guess, and only give it to the people you play with.
- Only the port `47100` is opened, and only `zmp-server` listens on it. It never runs the game, never receives
  files from players other than the game's state, and never stores anything outside its `data` folder.
- When you are done playing, close the server window, and remove the forwarding rule if you will not host again.

## 5. Browser players too

The same server and the same port also serve browser players (WebSocket). To let your friends play in the browser
without installing anything, start the server with the browser build: see [Browser mode](07-browser-mode.md).

## How many players and how far apart

Up to **6 players** per room, and several rooms at once on the same server. Lockstep waits for the slowest
connection, so the game feels best when everyone is in the same region (same country or neighbouring ones). The
server adapts the input delay to the connections every few seconds.
