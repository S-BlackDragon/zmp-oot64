# 8. Troubleshooting

ZMP's own messages in the game are in Spanish; each one is translated here.

## When joining a room

| Message | Meaning | What to do |
|---|---|---|
| "No se pudo conectar a ..." | Could not connect | Check the address and port; check that the host's server window is open; for internet play, that the port is forwarded ([guide](04-host-on-your-pc.md)) or that everyone is on the same [virtual LAN](05-host-with-a-virtual-lan.md). |
| "Rechazado por ... build" (build_mismatch) | Your game build is different from the room's | Everyone must use exactly the same release (same `soh.exe`, or for the browser the same page). Download the same zip. Desktop and browser players cannot share a room. |
| "Tu ROM es X y la sala usa Y" (oot_mismatch) | Your ROM version is different from the room's | Everyone uses the same ROM version ([Your ROM](02-your-rom.md)). |
| "Rechazado por ... cvar" | Your gameplay settings differ from the room's | Put the "Enhancements" / "Cheats" options back to their defaults. The quickest way: close the game, delete `shipofharkinian.json` in the game folder, open it again. |
| "Rechazado por ... name" (name_taken) | Someone in the room already uses that name | Pick another name. (If it is you after a crash, wait a few seconds and connect again with the same name to get your Link back.) |
| "El nombre debe tener entre 1 y 32 caracteres" | Name must be 1 to 32 characters | Type a shorter or non-empty name. |
| "Version de protocolo ..." (protocol_mismatch) | Game and server versions don't match | Update the server and the game to the same release. |
| "Esperando a ..." | Waiting for a player | Only appears when someone's connection really stops for more than a second. It goes away by itself, or that player drops. |
| "DESYNC: resincronizado" | The PCs disagreed and were corrected | Nothing; it fixes itself. If it happens often, send the logs (below). |

## Browser mode

| Problem | What to do |
|---|---|
| The page stays on "Downloading..." or "Preparing..." | Wait (the first load downloads a few tens of MB); then reload. Check the browser console (F12, Console). |
| "This file is compressed..." / "...isn't a supported version..." / "...doesn't match a known good one" | Choose the uncompressed `.z64`/`.n64`/`.v64` ROM; a modified or bad dump is refused. |
| "This page was opened over https..." warning | Use a `wss://` address, or open the page over `http` (for example from the host's `zmp-server -web`). |
| "Conectando..." never becomes "Conectado" | The address or port is wrong or blocked; an `https` page trying a `ws://` address is blocked by the browser. |
| "The game stopped unexpectedly" / tab crashes | Usually memory: each tab needs about 1 GB. Close other tabs and programs. |
| Opening `index.html` by double-click shows nothing | The page must be served by a web server (`zmp-server -web`, or `python -m http.server`). |

## Server

- **The window closes at once**: run it from a terminal to read the error. "address already in use" means another
  program (or another copy of the server) uses port 47100: close it, or use `-listen :47101` (and tell friends the
  new port).
- **Friends cannot reach it but it works on your PC**: the router forwarding or the firewall. Open
  `http://<your public address>:47100/healthz` from a phone on mobile data (not your Wi-Fi): it should answer
  `zmp-server ... protocol 4`. If it does not, the port is not reachable from the internet.
- **Starting a room over**: use a new room name, or have the room creator tick "Empezar la sala con mi partida" before
  connecting.

## Sending a bug report

Open an issue on GitHub with:

- what happened, at what time, and what you were doing;
- a screenshot;
- the `logs` folder of each player's game folder (`logs\zmp.log` is ZMP's own log);
- for browser players, the console text (F12, Console, right click, "Save as...");
- the server's log (its window, or `docker compose logs`).

Logs contain player names, the room name and the server address, and file paths that may include your Windows
user name: read them before you post them publicly.
