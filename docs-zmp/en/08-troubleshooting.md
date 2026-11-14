# 8. Troubleshooting

**English** | [Español](../es/08-troubleshooting.md)

The ZMP window shows its messages in your language (Spanish or English, see **Profile > Language**). The English text
is listed here; the [Spanish guide](../es/08-troubleshooting.md) lists the Spanish one.

## When joining a room

| Message | What to do |
|---|---|
| "Could not connect to ..." | The address or port is wrong, or the server cannot be reached. Check that the host is still hosting. Over the internet, look at the host's "From the internet" line ([main guide](00-host-and-send-the-code.md#what-the-from-the-internet-line-means)) and pick a plan B if needed. |
| "Wrong password" | Type the room password again (it is case sensitive). The host sees it in the Host tab, or on the page of the double-click server. |
| "This room has a password: type it and connect again" | Type it in **Password** (under By address) and connect again. A code with the password inside never asks. |
| "The host left a hint for the password: ..." | The code carries only a hint: type the password in **Password** and press **Join with this code** again. |
| "This server only lets you create rooms with a password: type one" | That room does not exist yet and this server wants a password: type one; it becomes the room's password. |
| "Too many failed attempts: wait 60 seconds" | Too many wrong passwords from your address. Wait, then type the right one. |
| "That server has no such room" | The server only hosts its own room (a game that is hosting, or a double-click server). Use the code, or check the room name. |
| "Your version of the game is not the room's (the room uses ..., you have ...): everyone needs the same version. Update the game." | Everyone must use exactly the same release (the same zip). Download the same version as the host. |
| "This room uses another engine (...): desktop with desktop, browser with browser" | Desktop and browser players cannot share a room. Use the same kind as the room. |
| "Your ROM is not the room's (the room uses ..., you ...): the same ROM version is needed" | Everyone uses the same ROM version ([Your ROM](02-your-rom.md)). |
| "Your game files (soh.o2r) are not the room's: reinstall the same version" | Unzip the same release again into a new folder. |
| "Your locked game settings are not the room's: use the same version" | Use the same version as the others, and put the "Enhancements" / "Cheats" options back to their defaults (quickest: close the game, delete `shipofharkinian.json` in the game folder, open it again). |
| "The room plays another randomizer seed (...)" | Everyone needs the same randomizer seed. |
| "Someone called ... is already in the room: change your name" | Pick another name in **Profile**. |
| "The room is full (6 players)" | Up to 6 players per room. |
| "This game and the server speak different protocol versions ..." | Update the older one: the game or the server. |
| "The name must be 1 to 32 characters long" / "The room must be 1 to 64 characters long" | Fix the name or the room. |
| "That address is not valid ..." | Accepted: `host`, `host:port`, an IP, `[IPv6]:port`, `ws://...`, `wss://...`. |
| "That address answers, but it is not a ZMP server" | The address points to something else (a web page, another program). Check the address and the port. |
| "The server's certificate is not valid: not connecting (wss)" | The `wss://` server has an expired, self-made or wrong certificate. Its owner must fix it ([wss guide](12-wss-for-browsers.md)). |
| "This server has its own password: type it under Advanced" / "The server password is wrong" | Type the server password in **Advanced > Server password (if it has one)**. |
| "Too many connections from your address" / "The server takes no more rooms" | Limits of that server (usually a relay). Try later or use another server. |
| "Joining took too long: connect again" | Connect again. |
| "The server could not prove it knows the room's password: it is not the room you want" | The server at that address is not the one that made the code. Ask the host for a new code. |
| "Connection to the server lost: retrying..." | The game reconnects by itself. If it lasts, the server stopped or the connection dropped. |

## Invite codes

| Message | What to do |
|---|---|
| "The code does not check out: make sure it is copied whole and without typos" | Copy it again, all of it. Dashes and spaces do not matter. |
| "The code is too short or too long" / "The code has a character that is not valid" | Part of the code is missing or something else got pasted. Copy it again. |
| "The code is from a newer version: update the game" | The host has a newer ZMP. Update. |

## When hosting

| Message | What to do |
|---|---|
| "zmp-server.exe is not next to the game: download the app again" | Unzip the whole release zip again; `zmp-server.exe` must be next to `soh.exe`. |
| "The server did not start: ..." / "The server closed (code ...)" | Usually the port is in use (another server, or a second copy of the game hosting). Close it, or choose another port (for example `47101`). The server's log is `zmp-host\zmp-server.log` in the game folder. |
| The "From the internet" line | See [what it means](00-host-and-send-the-code.md#what-the-from-the-internet-line-means). |
| Friends at home cannot see the game in "On your local network" | Windows Firewall must allow `soh.exe` and `zmp-server` on private networks, and everyone must be on the same network (some guest Wi-Fi networks keep devices apart). Pasting the code still works. |

## In the game

These notices are still only in Spanish:

| Notice | Meaning |
|---|---|
| "Esperando a que alguien cargue una partida para crear el grupo" | Waiting for someone to load a save to create the group. The host (or anyone) loads a save. |
| "Entrando en la partida del grupo..." | Entering the group's game. Wait a few seconds. |
| "Sin conexion con el grupo: esperando a ..." / "Esperando a ..." | Someone's connection stopped for more than a second. It goes away by itself, or that player drops. |
| "DESYNC: resincronizado" | The PCs disagreed and were corrected. Nothing to do; if it happens often, send the logs (below). |
| "Solo el anfitrion guarda la partida" | Only the player who created the group writes the save file. |

## Browser mode

| Problem | What to do |
|---|---|
| The page stays on "Downloading..." or "Preparing..." | Wait (the first load downloads a few tens of MB); then reload. Check the browser console (F12, Console). |
| "This file is compressed..." / "...isn't a supported version..." / "...doesn't match a known good one" | Choose the uncompressed `.z64`/`.n64`/`.v64` ROM; a modified or bad dump is refused. |
| "This page was opened over https..." warning | Use a `wss://` address, or open the page over `http` (for example from `zmp-server -web`). |
| A notice at the top of the page about `zmp-config.json` | The site's configuration has a value the game does not accept; that value is ignored and the rest works. The site's owner can fix it ([guide](10-web-version-on-your-site.md)). |
| "The game stopped unexpectedly" / tab crashes | Usually memory: each tab needs about 1 GB. Close other tabs and programs. |
| Opening `index.html` by double-click shows nothing | The page must be served by a web server (`zmp-server -web`, or `python -m http.server`). |

## Server

- **The console closes at once**: run it from a terminal to read the error. "address already in use" means another
  program (or another copy of the server) uses port 47100: close it, or use another port (`listen = ":47101"` in
  `zmp-server.toml`, or `-listen :47101`). The double-click server waits for Enter before closing when it cannot
  start.
- **Friends cannot reach it but it works on your PC**: the router or the firewall. Open
  `http://<your public address>:47100/healthz` from a phone on mobile data (not your Wi-Fi): it should answer
  `zmp-server ... protocol 5`. If it does not, the port is not reachable from the internet
  ([open it by hand](09-open-the-port-by-hand.md), or another plan B).
- **Starting a room over**: use a new room name, or tick "Empezar la sala con mi partida" in **Advanced** before
  connecting.

## Sending a bug report

ZMP never sends anything by itself: logs stay on your PC until you send them. Open an issue on GitHub with:

- what happened, at what time, and what you were doing;
- a screenshot;
- the `logs` folder of each player's game folder (`logs\zmp.log` is ZMP's own log);
- when hosting from the game, `zmp-host\zmp-server.log`; for another server, its console or `docker compose logs`;
- for browser players, the console text (F12, Console, right click, "Save as...").

Logs contain player names, the room name, server addresses, and file paths that may include your Windows user name:
read them before you post them publicly. Never post an invite code that carries a password.
