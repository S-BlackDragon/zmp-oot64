# 7. Browser mode

**English** | [Español](../es/07-browser-mode.md)

ZMP also runs in a web browser: the same game and the same multiplayer, compiled to WebAssembly. Nothing to install,
and it works on any desktop OS with a recent **Chrome, Edge or Firefox**. Phones and tablets are not supported.

Things to know:

- **Browser players play with browser players.** A browser build and the desktop `soh.exe` are different builds, so
  they cannot share a room. One server can hold both kinds of rooms at once.
- Everyone in a browser room must load **the same web build** (the same `soh.wasm`). The easiest way to guarantee it
  is that everybody opens the page from the same place (the host's server).
- Each tab uses about **1 GB of memory**. One tab per player.
- The page asks for **your ROM each time** you open it. It is read from your disk and converted in the tab's memory
  (about a minute). Nothing is uploaded and nothing from the ROM is stored in the browser. Everyone must use the same
  ROM version, as on desktop.
- Saves of the room live on the server, as on desktop. The browser keeps only the game's settings.

## The easy way: the host's server serves the page

1. The host downloads `ZMP-<version>-web.zip` from the release (or [builds it](build.md)) and unzips it into a folder,
   for example `C:\Games\ZMP-web` (it contains `index.html`, `soh.js`, `soh.wasm`...).
2. The host starts the server with that folder, from a terminal in the server's folder:

   ```
   zmp-server.exe -web C:\Games\ZMP-web
   ```

   The log says `serving the browser game from C:\Games\ZMP-web on the same port`. (Docker: see
   [Host on a VPS](06-host-on-a-vps.md), section 4.)
3. Everyone opens `http://<server address>:47100/` in the browser: the host can use `http://127.0.0.1:47100/`, friends
   use the address they would type in the desktop game (public address with
   [port forwarding](04-host-on-your-pc.md), or the [Tailscale/ZeroTier](05-host-with-a-virtual-lan.md) address).
4. The page shows **Join a game**: the server address is already filled in (it is the page's own server). Type the
   **room** and **your name**.
5. Click **Choose ROM file** (or drop your ROM on the page). After about a minute the game starts and connects to the
   room by itself.

The top line of the game (if the diagnostic line is on) says "Conectado a ws://... (sala <room>, N jugadores)".

## Other ways to serve the page

The browser build is a plain static website: any web server can serve the folder, for example
`python -m http.server 8080` inside it (then open `http://127.0.0.1:8080/`). In that case the server field is empty:
type the room server's address yourself (`host:port`, for example `192.168.0.20:47100`).

The page must be opened through a web server (`http://` or `https://`), not by double-clicking `index.html`
(`file://` does not work).

### http, https and wss

- A page opened over **http** can connect to any server address (`host:port`).
- A page opened over **https** can only use secure WebSockets: the server must be reachable at a `wss://` address
  (for example behind the Caddy proxy of [Host on a VPS](06-host-on-a-vps.md)). Type the full address, like
  `wss://play.example.org/`. The page warns you if what you typed will be blocked.

### Server address formats the page accepts

`host` (port 47100), `host:port`, `[IPv6]:port`, or a full `ws://...` / `wss://...` address. Leave it empty to play
alone; you can still connect later from the game's menu (Esc, Network, ZMP), as on desktop.

### Link with everything filled in

The page also reads the address from its URL, so the host can send a ready link:
`http://<page address>/?host=<server>&port=47100&room=<room>&name=<name>` (the name part is better left out, so
each friend types their own).
