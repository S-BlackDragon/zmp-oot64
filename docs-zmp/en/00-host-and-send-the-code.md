# Press Host game and send the code to your friend

**English** | [Español](../es/00-host-and-send-the-code.md)

This is the easy way to play with friends, and the one to try first. One of you (the **host**) presses a button in
the game, gets a short **invite code**, and sends it to the others. They paste it and they are in. No accounts, no
separate program to set up: the game includes everything.

Before you start, everyone needs:

- **the same version of ZMP** (the same `ZMP-<version>-windows.zip`; [Get the game](01-get-the-game.md));
- **the same version of the ROM** ([Your ROM](02-your-rom.md));
- the host needs a **Windows PC**. Friends can be in the same house or anywhere on the internet.

## The ZMP window

Everything happens in the **"Sala ZMP"** window (the ZMP room window). It opens by itself the first time you start
the game. If you closed it: press **Esc**, go to **Network > ZMP** and press **"Mostrar u ocultar ventana de sala"**
(show or hide the room window).

It has four tabs: **Join**, **Host**, **Profile** and **Advanced**. Its language follows your Windows language
(Spanish or English); you can change it in **Profile > Language**.

## Host side

1. Open `soh.exe` and stay on the file select screen.
2. **Profile** tab: type your name and press **Change name**. Each player needs a different name; it is shown over
   your Link.
3. **Host** tab, section **Host a game**:
   - **Who can join**: leave **"My local network and whoever has the code"**. (The other choice, "This PC only",
     is for tests: nobody else can get in.)
   - **Room** and **Room password**: leave them empty and the game makes up a room name and a password. Type a room
     name of your own if you want to come back to the same game another day (see
     [Saving](03-play.md#saving)).
   - **Port**: leave `47100`.
   - **"Keep hosting after the game closes"**: leave it off (then closing the game also stops hosting).
4. Press **Host and play**.
5. The first time, **Windows Firewall** may ask whether to let `zmp-server` (and `soh.exe`) use the network: press
   **Allow access**. If your home network is set as "Public" in Windows, tick public networks too.
6. After a few seconds the tab shows:
   - "Hosting room ... on port 47100" and the **room password**;
   - **Invite code (give it to your friends)**: a code like `ZMP-0410-416B-01RG-...`, with **Copy code** and
     **Copy link** buttons;
   - **From the internet**: whether friends on the internet can reach you (see below);
   - **Players**: how many are in.
7. Press **Copy code** and send the code to your friends (chat, mail, message). The code carries your address, the
   room and its password: **whoever has it can join**, so send it only to the people you play with.
8. Load your save file (or start a new one). That save becomes the group's game. Until then the top of the screen
   says "Esperando a que alguien cargue una partida para crear el grupo" (waiting for someone to load a save to
   create the group).

To end: press **Stop hosting**, or close the game. Either way the game stops its server and removes the rule it
asked the router for.

## Friend side

1. Open `soh.exe` (the same version as the host) and stay on the file select screen. **Do not load a save.**
2. **Profile** tab: type your name and press **Change name**.
3. **Join** tab, section **Join with a code**: paste the code in the box and press **Join with this code**.
4. The tab says "Valid code: room ... at ...", then "Connecting to ...", then "Connected to ... (room ..., N
   players)". When the host's game is running you appear in the world next to the group (the screen says "Entrando
   en la partida del grupo...", entering the group's game).

If something goes wrong, a red line says why (wrong password, different version...). Each message is explained in
[Troubleshooting](08-troubleshooting.md).

### In the same house

Friends on the same home network also see the game in **Join > On your local network**, with the room, the host's
PC name, the players and whether it has a password. To join from that list, type the room password (the host sees
it in the Host tab) in the **Password** field under **By address**, then press **Join** in that row. Pasting the
code works too, and needs nothing else.

### Links instead of codes

**Copy link** gives a `zmp://join/...` link. It opens the game only on PCs where it has been allowed: in **Advanced**,
press **"Open zmp:// links with this game"** once (it is set for your Windows user only). From then on, clicking a
`zmp://` link opens the game and joins the room; if the game is already open, the link goes to that open game. Some
chat apps do not make `zmp://` links clickable: then just copy the code.

## What the "From the internet" line means

The game asks your router to open the port (with UPnP, NAT-PMP or PCP, which most home routers have) and then says
honestly what happened:

| The line says | What it means | What to do |
|---|---|---|
| "Asking the router..." | Still waiting for the router's answer. | Wait a few seconds. |
| "Opened by UPnP (or PCP, NAT-PMP): your public address is ..., port 47100." | The router opened the port and your address is a real internet address. | Send the code. If a friend still cannot get in, it is usually Windows Firewall: allow `zmp-server` on private and public networks. |
| "Reachable from the internet: a player already joined from outside your network." | Proven: someone joined over the internet. | Nothing. Enjoy. |
| "Possible CGNAT: the router opened the port by ..., but its external address (...) is not public." | Your internet provider (or a second router) shares your connection, so the internet probably cannot reach you. | Friends at home can still join. For the others, use a plan B (below). |
| "Router without UPnP: it did not answer UPnP, PCP or NAT-PMP." | The router did not help (UPnP may be turned off in its settings). | Turn UPnP on in the router's settings and host again, or use a plan B. |
| "The router answered but did not open the port (...)." | The router refused, often because another device already uses that port. | Try another port (for example `47101`) and host again, or use a plan B. |
| "Only this PC can join (listening on 127.0.0.1)." | You chose "This PC only". | Choose "My local network and whoever has the code". |

The game can only know what your router tells it. "Opened" is a very good sign; the line turns into "Reachable from
the internet" the moment a friend really joins from outside.

Below the line, "From a browser on your network: http://192.168.1.20:47100/" shows your PC's address on your home
network (useful for some plan B guides).

## If friends on the internet cannot reach you: plan B

Pick one; they all work with the same game:

- [Open the port by hand on your router](09-open-the-port-by-hand.md) (when the router has no UPnP).
- [Tailscale or ZeroTier](05-host-with-a-virtual-lan.md): a private network over the internet; works behind CGNAT,
  nothing to change on the router.
- [A relay](11-relay.md): a `zmp-server` someone runs on the internet; you make your room there.
- [A VPS with Docker](06-host-on-a-vps.md): a small rented server, always on.
- [A PC that only hosts](04-host-on-your-pc.md): the server alone, started with a double click, with its own page.

## Privacy

- ZMP uploads nothing and has no accounts. The game only connects to the server you type or that the code carries.
- To open the port, the game talks only to **your own router**; your public address comes from the router, not from
  any outside service.
- Hosting announces your room on **your local network only** (that is how "On your local network" works).
- Your logs stay on your PC: the `logs` folder of the game and, when you host, `zmp-host\zmp-server.log`. Nothing is
  sent anywhere unless you send it yourself.
