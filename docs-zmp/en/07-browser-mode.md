# 7. Browser mode

**English** | [Español](../es/07-browser-mode.md)

ZMP also runs in a web browser: the same game and the same multiplayer, compiled to WebAssembly. Nothing to install,
and it works on any desktop OS with a recent **Chrome, Edge or Firefox**. Phones and tablets are not supported.

Things to know:

- **Browser players play with browser players.** A browser build and the desktop `soh.exe` are different builds, so
  they cannot share a room ("This room uses another engine: desktop with desktop, browser with browser"). One server
  can hold both kinds of rooms at once.
- **A browser cannot host.** Browser players need a server: one that serves the page (below), a
  [double-click server](04-host-on-your-pc.md), a [VPS](06-host-on-a-vps.md) or a [relay](11-relay.md).
- Everyone in a browser room must load **the same web build** (the same `soh.wasm`). The easiest way to guarantee it
  is that everybody opens the page from the same place.
- Each tab uses about **1 GB of memory**. One tab per player.
- The page asks for **your ROM each time** you open it. It is read from your disk and converted in the tab's memory
  (about a minute). Nothing is uploaded and nothing from the ROM is stored in the browser. Everyone must use the same
  ROM version, as on desktop.
- The room's progress lives on the server, as on desktop. The browser keeps only the game's settings.

## The easy way: the server serves the page

1. Whoever hosts downloads `ZMP-<version>-web.zip` from the release (or [builds it](build.md)) and unzips it into a
   folder, for example `C:\Games\ZMP-web` (it contains `index.html`, `soh.js`, `soh.wasm`...).
2. They start the server with that folder, from a terminal in the server's folder:

   ```
   zmp-server.exe -web C:\Games\ZMP-web
   ```

   The log says `serving the browser game from C:\Games\ZMP-web on the same port`. (Docker: see
   [VPS](06-host-on-a-vps.md), section 6.) Friends on the internet need the port reachable, as for any server:
   [open it by hand](09-open-the-port-by-hand.md) or use [Tailscale/ZeroTier](05-host-with-a-virtual-lan.md).
3. Everyone opens `http://<server address>:47100/` in the browser: the host can use `http://127.0.0.1:47100/`.
4. The page shows **Join a game**: the server address is already filled in (it is the page's own server). Type the
   **room** and **your name**.
5. Click **Choose ROM file** (or drop your ROM on the page). After about a minute the game starts and joins the room
   by itself.

For a room with a password: open the game's ZMP window (**Esc > Network > ZMP**), **Join** tab, type it in
**Password** under **By address** and press **Connect**. Or put it in the page's address: `.../#pw=<password>`.

## Join with an invite code

Invite codes work in the browser too:

- open the page with the code after `#invite=`, for example `http://<page address>/#invite=ZMP-0410-416B-...`: the
  game joins with it as soon as the ROM is ready;
- or, once the game is running, paste the code in the ZMP window: **Join > Join with a code**.

What follows `#` in an address never reaches any server, and the page wipes it from the address bar once read.

## Other ways to serve the page

The browser build is a plain static website: any web server can serve the folder, for example
`python -m http.server 8080` inside it (then open `http://127.0.0.1:8080/`). In that case the server field is empty:
type the room server's address yourself (`host:port`, for example `192.168.1.20:47100`).

The page must be opened through a web server (`http://` or `https://`), not by double-clicking `index.html`
(`file://` does not work).

To preset your server, the room or the password policy for everyone who opens your page, add a `zmp-config.json`
next to it: [Put the web version on your site](10-web-version-on-your-site.md).

### http, https and wss

- A page opened over **http** can connect to any server address (`host:port`).
- A page opened over **https** can only use secure WebSockets: the server must be reachable at a `wss://` address.
  Type the full address, like `wss://zmp.example.org/`. The page warns you if what you typed will be blocked. How to
  set one up: [wss:// for browsers](12-wss-for-browsers.md).

### Server address formats the page accepts

`host` (port 47100), `host:port`, `[IPv6]:port`, or a full `ws://...` / `wss://...` address. Leave it empty to play
alone; you can still connect later from the game's ZMP window (Esc, Network, ZMP), as on desktop.

### Link with everything filled in

The page also reads its address: `http://<page address>/?server=<server>&room=<room>` fills in the server and the
room and joins as soon as the ROM is ready (`?host=<server>&port=47100` also works). Leave the name out, so each
friend types their own.

## Older browser builds

A browser build of ZMP 0.6.x (protocol 4) can still join rooms **without** a password on a 0.7.0 server, with other
players of that same build. Rooms with a password need 0.7.0.
